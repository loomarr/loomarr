package scan

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/provision"
)

// ServerCollection is one of the media server's collections, as the cutover imports it.
type ServerCollection struct {
	ID   string
	Name string
	// FromTMDB marks a collection a tool generated from a TMDB collection: the Library's ready-made
	// sets already hold those, so only one a record names is imported.
	FromTMDB bool
}

// ReferenceStore is what the ReferenceRewriter reads and writes.
type ReferenceStore interface {
	store.LibraryReferenceStore
	store.LibraryIDMapStore
	store.LibraryCollectionStore
	store.LibraryItemFactsReader
	store.LibraryTitleReader
	GetSetting(ctx context.Context, key string) (string, error)
	SetSetting(ctx context.Context, key, value string) error
}

// handMadeImportedMarker records that the media server's hand-made collections were imported, so
// one the household deletes afterwards is not brought back.
const handMadeImportedMarker = "migration.library.collections_import.v1"

// ReferenceRewriter points the item and collection ids stored records name at the Library's own
// (#2151 step 15b), after each id map pass. Every reader takes either kind, so what maps is
// rewritten at once and what doesn't waits for a later pass; nothing is held back for it.
//
// The media server's collections become household collections: the hand-made ones once, and any
// other one a record names when it is first named. Their titles come over through the id map.
type ReferenceRewriter struct {
	Store ReferenceStore
	// Collections lists the media server's collections; nil, or ErrNoServerCatalog, imports none.
	Collections func(ctx context.Context) ([]ServerCollection, error)
	// Members lists one collection's titles by their media-server ids.
	Members func(ctx context.Context, collectionID string) ([]string, error)
	// Flavor names the media server collections are imported from (emby, jellyfin).
	Flavor func() string
	Now    func() time.Time
	Log    *slog.Logger
}

// ReferenceReport is what one pass did.
type ReferenceReport struct {
	Rewritten int
	Imported  int
	// UnmappedItems and UnmappedCollections are stored ids neither the Library's nor mapped yet.
	UnmappedItems, UnmappedCollections []string
}

// Rewrite runs one pass.
func (r *ReferenceRewriter) Rewrite(ctx context.Context) (ReferenceReport, error) {
	var report ReferenceReport
	refs, err := r.Store.LibraryReferences(ctx)
	if err != nil {
		return report, err
	}
	idMap, err := r.Store.LibraryIDMap(ctx)
	if err != nil {
		return report, err
	}
	items := make(map[string]string, len(idMap))
	for _, m := range idMap {
		if m.ItemID != "" {
			items[m.MediaID] = m.ItemID
		}
	}
	library, err := r.Store.LibraryItemFacts(ctx, refs.Items)
	if err != nil {
		return report, err
	}
	if err := r.mapByTitle(ctx, refs, library, items); err != nil {
		return report, err
	}
	imported, err := r.importCollections(ctx, refs.Collections, items)
	if err != nil {
		return report, err
	}
	report.Imported = imported
	owned, err := r.Store.ListLibraryCollections(ctx)
	if err != nil {
		return report, err
	}
	collections := map[string]string{}
	household := map[string]bool{}
	for _, c := range owned {
		household[c.ID] = true
		if c.ImportedID != "" {
			collections[c.ImportedID] = c.ID
		}
	}
	if report.Rewritten, err = r.Store.RewriteLibraryReferences(ctx, items, collections); err != nil {
		return report, err
	}

	for _, id := range refs.Items {
		if _, ok := library[id]; !ok && items[id] == "" {
			report.UnmappedItems = append(report.UnmappedItems, id)
		}
	}
	for _, id := range refs.Collections {
		if !household[id] && collections[id] == "" {
			report.UnmappedCollections = append(report.UnmappedCollections, id)
		}
	}
	r.log().Info("library references: pass done", "rewritten", report.Rewritten, "imported", report.Imported,
		"unmapped_items", len(report.UnmappedItems), "unmapped_collections", len(report.UnmappedCollections))
	return report, nil
}

// mapByTitle maps a movie or series id the id map doesn't pair through the title it was recorded
// for: a media server that re-added its library gave every item a new id, and the old one names
// nothing, but its title's provider ids still name the Library's.
func (r *ReferenceRewriter) mapByTitle(
	ctx context.Context, refs store.LibraryReferences, library map[string]store.LibraryItemFacts, items map[string]string,
) error {
	for id, key := range refs.Keys {
		if _, ok := library[id]; ok || items[id] != "" {
			continue
		}
		mt, provider, value, ok := provision.ParseKey(key)
		if !ok {
			continue
		}
		kind := "movie"
		if mt == provision.Series {
			kind = "series"
		}
		title, found, err := r.Store.LibraryTitleByProviderID(ctx, kind, provider, strconv.Itoa(value))
		if err != nil {
			return err
		}
		if found {
			items[id] = title.ID
		}
	}
	return nil
}

// importCollections brings the media server's collections over that are due: every hand-made one
// on the first pass, and any a record names that no household collection came from yet.
func (r *ReferenceRewriter) importCollections(ctx context.Context, named []string, items map[string]string) (int, error) {
	if r.Collections == nil || r.Members == nil {
		return 0, nil
	}
	server, err := r.Collections(ctx)
	if errors.Is(err, ErrNoServerCatalog) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("list the media server's collections: %w", err)
	}
	owned, err := r.Store.ListLibraryCollections(ctx)
	if err != nil {
		return 0, err
	}
	have := map[string]bool{}
	for _, c := range owned {
		if c.ImportedID != "" {
			have[c.ImportedID] = true
		}
	}
	isNamed := map[string]bool{}
	for _, id := range named {
		isNamed[id] = true
	}
	_, err = r.Store.GetSetting(ctx, handMadeImportedMarker)
	firstPass := errors.Is(err, store.ErrNotFound)
	if err != nil && !firstPass {
		return 0, err
	}
	imported := 0
	for _, c := range server {
		due := isNamed[c.ID] || (firstPass && !c.FromTMDB)
		if have[c.ID] || !due {
			continue
		}
		members, err := r.Members(ctx, c.ID)
		if err != nil {
			return imported, fmt.Errorf("list collection %s: %w", c.ID, err)
		}
		titles := make([]string, 0, len(members))
		for _, id := range members {
			if item := items[id]; item != "" {
				titles = append(titles, item)
			}
		}
		if _, err := r.Store.CreateLibraryCollection(ctx, store.LibraryCollection{
			Name: c.Name, ImportedFrom: r.flavor(), ImportedID: c.ID, Members: titles,
		}, r.now()); err != nil {
			return imported, fmt.Errorf("import collection %s: %w", c.ID, err)
		}
		imported++
	}
	if firstPass {
		if err := r.Store.SetSetting(ctx, handMadeImportedMarker, "true"); err != nil {
			return imported, err
		}
	}
	return imported, nil
}

func (r *ReferenceRewriter) flavor() string {
	if r.Flavor != nil {
		return r.Flavor()
	}
	return ""
}

func (r *ReferenceRewriter) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

func (r *ReferenceRewriter) log() *slog.Logger {
	if r.Log != nil {
		return r.Log
	}
	return slog.Default()
}
