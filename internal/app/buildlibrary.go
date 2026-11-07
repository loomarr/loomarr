package app

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/api"
	"github.com/loomarr/loomarr/internal/diagnostics"
	"github.com/loomarr/loomarr/internal/inventory"
	"github.com/loomarr/loomarr/internal/library"
	"github.com/loomarr/loomarr/internal/library/arrhook"
	"github.com/loomarr/loomarr/internal/library/scan"
	"github.com/loomarr/loomarr/internal/media/header"
	"github.com/loomarr/loomarr/internal/metadataprovider/arr"
	"github.com/loomarr/loomarr/internal/metadataprovider/nfo"
	"github.com/loomarr/loomarr/internal/metadataprovider/precedence"
	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/requester"
	"github.com/loomarr/loomarr/internal/settings"
	"github.com/loomarr/loomarr/internal/store"
)

// libraryStore is what Settings → Library and the scanner keep their state in.
type libraryStore interface {
	store.LibraryRootStore
	store.LibraryFileStore
	inventory.Repository
}

// libraryBuild is what Settings → Library needs beyond the store and settings.
type libraryBuild struct {
	lib *library.Client
	// budget is playout's admission ledger; header reads wait while it holds a session. Nil
	// without internal playout, when nothing on this box needs the disks first.
	budget *playout.ResourceBudget
	// ffmpegPath locates ffprobe, the header reader's fallback.
	ffmpegPath string
	// processes lists each ffprobe run in Diagnostics; nil lists none.
	processes *diagnostics.ProcessManager
	// hookSecret is the password Sonarr and Radarr send with each webhook.
	hookSecret string
	log        *slog.Logger
	owner      *generationLifecycle
}

// libraryServices is Settings → Library and the arrs' webhooks; both nil without a store.
type libraryServices struct {
	roots api.LibraryRootService
	hooks api.LibraryHookService
}

// buildLibrary wires Settings → Library (#2151 decision 6) and starts the scanner, its folder
// watcher and the arr webhook registrations for the generation's lifetime. Sources, the path map,
// the arr connections, Loomarr's address and the kill switch are read per call, so a saved
// setting applies at once.
func buildLibrary(st libraryStore, set resolved, deps libraryBuild) libraryServices {
	if st == nil {
		return libraryServices{}
	}
	pathMap := func() library.PathMap { return library.ParsePathMap(settings.LibraryPathMap.Get(set)) }
	roots := &scan.Roots{
		Store:   st,
		Sources: func() []scan.Source { return libraryRootSources(set, deps.lib) },
		PathMap: pathMap,
		Disk:    scan.OSDisk{},
		Now:     time.Now,
	}
	resolver, err := precedence.New("US")
	if err != nil {
		deps.log.Error("library scan: not started", "err", err)
		return libraryServices{roots: &scan.Library{Roots: roots}}
	}
	conns := set.arrConns()
	managers := []scan.Manager{
		{Authority: "sonarr", Lister: arr.NewSonarr(conns.Sonarr, nil)},
		{Authority: "radarr", Lister: arr.NewRadarr(conns.Radarr, nil)},
	}
	im := &scan.Importer{
		Store:     st,
		Inventory: inventory.New(st),
		Disk:      scan.OSDisk{},
		Managers:  func() []scan.Manager { return managers },
		NFO:       nfo.New(),
		Resolver:  resolver,
		Header:    libraryHeader(playout.FFprobeSourceNextTo(deps.ffmpegPath, deps.processes)),
		Enabled:   func() bool { return settings.LibraryScannerEnabled.Get(set) },
		Now:       time.Now,
		Log:       deps.log,
	}
	if deps.budget != nil {
		im.Busy = func() bool { return deps.budget.Sessions() > 0 }
	}
	scanner := scan.NewScanner(im, scan.ReconcileInterval)
	watcher := &scan.Watcher{Scanner: scanner}
	hooks := &scan.ArrHooks{
		Scanner: scanner,
		Clients: map[string]arrhook.Client{
			"sonarr": {Conn: arrhook.Conn(conns.Sonarr)},
			"radarr": {Conn: arrhook.Conn(conns.Radarr)},
		},
		URL:    func(app string) string { return libraryHookURL(settings.ServerPublicURL.Get(set), app) },
		Secret: deps.hookSecret,
	}
	deps.owner.goRunAfterBuild(scanner.Run)
	deps.owner.goRunAfterBuild(watcher.Run)
	deps.owner.goRunAfterBuild(hooks.Run)
	return libraryServices{roots: &scan.Library{Roots: roots, Scanner: scanner}, hooks: hooks}
}

// libraryHookURL is where app posts its events: Loomarr's own address as the arrs reach it, the
// same one the media server and Tunarr use. "" when that address is unset.
func libraryHookURL(publicURL, app string) string {
	base := strings.TrimRight(strings.TrimSpace(publicURL), "/")
	if base == "" {
		return ""
	}
	return base + "/v1/library/hooks/" + app
}

// libraryHeader reads a file's header natively, and through ffprobe when the container needs it.
func libraryHeader(probe playout.SourceProber) func(context.Context, string) (header.Header, error) {
	fallback := func(ctx context.Context, path string) (header.Header, error) {
		observed, err := probe(ctx, path)
		if err != nil {
			return header.Header{}, err
		}
		facts := inventoryFactsOf(observed)
		return header.Header{
			Container: facts.Container, DurationMillis: facts.DurationMillis, Bitrate: facts.Bitrate,
			Streams: facts.Streams,
		}, nil
	}
	return func(ctx context.Context, path string) (header.Header, error) {
		h, _, err := header.Probe(ctx, path, fallback)
		return h, err
	}
}

// libraryRootSources is every authority that can name a library folder: the media server when
// its connection is complete, then Sonarr and Radarr whatever requester is chosen (decision 2).
func libraryRootSources(set resolved, lib *library.Client) []scan.Source {
	var sources []scan.Source
	if lib != nil && set.libraryConfigured() {
		if name, err := lib.FlavorName(); err == nil {
			sources = append(sources, scan.Source{Authority: name, Folders: lib.MediaFolders})
		}
	}
	arrs := requester.NewArr(set.arrConns())
	for _, app := range []string{"sonarr", "radarr"} {
		sources = append(sources, scan.Source{Authority: app, Folders: func(ctx context.Context) ([]string, error) {
			folders, err := arrs.RootFolders(ctx, app)
			if errors.Is(err, requester.ErrArrNotConfigured) {
				return nil, scan.ErrSkipped
			}
			return folders, err
		}})
	}
	return sources
}
