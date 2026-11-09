package scan

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/loomarr/loomarr/internal/library"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

// IDMapInterval is how often the IDMapper pairs the media server's ids again. A pass lists the whole
// catalog, so it runs rarely; the cutover reads the latest pass.
const IDMapInterval = 6 * time.Hour

// IDMap pairs each media-server movie and episode id with the library item its file was written as
// (#2151 step 15a), by file path through the derived path map, as the parity gate pairs them. A
// series pairs through its episodes: the one show they were all written under. An id that does not
// pair is kept with an empty ItemID and the reason. The result is ordered by media id.
func IDMap(server []library.ServerFile, files []librarystore.LibraryFile, pathMap library.PathMap) []store.LibraryIDMapping {
	byPath := make(map[string]librarystore.LibraryFile, len(files))
	for _, f := range files {
		if f.MissingAt.IsZero() {
			byPath[f.Path] = f
		}
	}
	type pairing struct {
		kind   string
		items  []string
		reason string
	}
	titles := map[string]*pairing{}
	shows := map[string]*pairing{}
	for _, s := range server {
		p := titles[s.ItemID]
		if p == nil {
			p = &pairing{kind: s.Kind}
			titles[s.ItemID] = p
		}
		file, reason := pairedFile(s, byPath, pathMap)
		if reason != "" {
			p.reason = cmp.Or(p.reason, reason)
			continue
		}
		if !slices.Contains(p.items, file.ItemID) {
			p.items = append(p.items, file.ItemID)
		}
		if s.Kind != "episode" || s.SeriesItemID == "" || file.SeriesID == "" {
			continue
		}
		show := shows[s.SeriesItemID]
		if show == nil {
			show = &pairing{kind: "series"}
			shows[s.SeriesItemID] = show
		}
		if !slices.Contains(show.items, file.SeriesID) {
			show.items = append(show.items, file.SeriesID)
		}
	}
	out := make([]store.LibraryIDMapping, 0, len(titles)+len(shows))
	add := func(id string, p *pairing, split string) {
		m := store.LibraryIDMapping{MediaID: id, Kind: p.kind}
		switch len(p.items) {
		case 0:
			m.Reason = p.reason
		case 1:
			m.ItemID = p.items[0]
		default:
			m.Reason = fmt.Sprintf(split, len(p.items))
		}
		out = append(out, m)
	}
	for id, p := range titles {
		add(id, p, "its versions are %d different titles")
	}
	for id, p := range shows {
		if _, both := titles[id]; both {
			continue // a media-server id is one item; a show is never also a movie or episode
		}
		add(id, p, "its episodes are in %d different shows")
	}
	slices.SortFunc(out, func(a, b store.LibraryIDMapping) int { return cmp.Compare(a.MediaID, b.MediaID) })
	return out
}

// pairedFile is the scanner's file for one media-server file, or why there is none.
func pairedFile(s library.ServerFile, byPath map[string]librarystore.LibraryFile, pathMap library.PathMap) (librarystore.LibraryFile, string) {
	local, mapped := pathMap.Apply(s.Path)
	file, found := byPath[local]
	switch {
	case !mapped:
		return file, "the path map doesn't cover its file"
	case !found:
		return file, "the scanner hasn't found its file"
	case file.Problem != "":
		return file, "the scanner skips its file: " + file.Problem
	case file.ItemID == "":
		return file, "its file isn't a title yet"
	}
	return file, ""
}

// ErrNoServerCatalog is what ServerFiles answers when no media server is connected: there is no
// catalog to pair, and nothing failed.
var ErrNoServerCatalog = errors.New("no media server is connected")

// IDMapStore is what the IDMapper reads and writes.
type IDMapStore interface {
	ParityStore
	store.LibraryIDMapStore
}

// IDMapper keeps the cutover's id map (#2151 step 15a) while the media server still lists the
// catalog. A pass that cannot list it leaves the last map in place.
type IDMapper struct {
	Store IDMapStore
	// ServerFiles lists the media server's movie and episode files.
	ServerFiles func(ctx context.Context) ([]library.ServerFile, error)
	// Enabled is the scanner's kill switch; nil is on.
	Enabled func() bool
	// References rewrites stored ids through each new map; nil leaves them.
	References *ReferenceRewriter
	Interval   time.Duration
	Now        func() time.Time
	Log        *slog.Logger
}

// Run pairs until ctx ends.
func (m *IDMapper) Run(ctx context.Context) {
	tick := time.NewTicker(orDuration(m.Interval, IDMapInterval))
	defer tick.Stop()
	for {
		if err := m.Tick(ctx); err != nil && ctx.Err() == nil {
			m.log().Warn("library id map: pass failed", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

// Tick runs one pass: it lists the media server's files and the scanner's, pairs them, and
// replaces the map.
func (m *IDMapper) Tick(ctx context.Context) error {
	if (m.Enabled != nil && !m.Enabled()) || m.ServerFiles == nil {
		return nil
	}
	server, err := m.ServerFiles(ctx)
	switch {
	case errors.Is(err, ErrNoServerCatalog):
		return nil
	case err != nil:
		return fmt.Errorf("list the media server's files: %w", err)
	case len(server) == 0:
		// An empty catalog is far likelier a media server mid-rebuild than a library with nothing
		// in it; the last map stands.
		return nil
	}
	roots, pathMap, err := rootsInUse(ctx, m.Store)
	if err != nil {
		return err
	}
	var files []librarystore.LibraryFile
	for _, root := range roots {
		rows, err := m.Store.LibraryFiles(ctx, root)
		if err != nil {
			return err
		}
		files = append(files, rows...)
	}
	mappings := IDMap(server, files, pathMap)
	if err := m.Store.ReplaceLibraryIDMap(ctx, mappings, m.now()); err != nil {
		return err
	}
	unpaired := 0
	for _, mapping := range mappings {
		if mapping.ItemID == "" {
			unpaired++
		}
	}
	m.log().Info("library id map: pass done", "ids", len(mappings), "unpaired", unpaired)
	if m.References != nil {
		if _, err := m.References.Rewrite(ctx); err != nil {
			return fmt.Errorf("rewrite library references: %w", err)
		}
	}
	return nil
}

func (m *IDMapper) now() time.Time {
	if m.Now != nil {
		return m.Now()
	}
	return time.Now()
}

func (m *IDMapper) log() *slog.Logger {
	if m.Log != nil {
		return m.Log
	}
	return slog.Default()
}
