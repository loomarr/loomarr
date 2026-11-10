package app

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"time"

	libraryapi "github.com/loomarr/loomarr/internal/api/library"
	"github.com/loomarr/loomarr/internal/app/playoutresolve"
	"github.com/loomarr/loomarr/internal/library"
	"github.com/loomarr/loomarr/internal/library/arrhook"
	"github.com/loomarr/loomarr/internal/library/inventory"
	mp "github.com/loomarr/loomarr/internal/library/metadataprovider"
	"github.com/loomarr/loomarr/internal/library/metadataprovider/arr"
	"github.com/loomarr/loomarr/internal/library/metadataprovider/localart"
	"github.com/loomarr/loomarr/internal/library/metadataprovider/nfo"
	"github.com/loomarr/loomarr/internal/library/metadataprovider/precedence"
	mptmdb "github.com/loomarr/loomarr/internal/library/metadataprovider/tmdb"
	"github.com/loomarr/loomarr/internal/library/requester"
	"github.com/loomarr/loomarr/internal/library/scan"
	"github.com/loomarr/loomarr/internal/library/tmdb"
	"github.com/loomarr/loomarr/internal/media/header"
	"github.com/loomarr/loomarr/internal/media/images"
	"github.com/loomarr/loomarr/internal/platform/diagnostics"
	"github.com/loomarr/loomarr/internal/platform/settings"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
	"github.com/loomarr/loomarr/internal/playout"
)

// libraryStore is what Settings → Library and the scanner keep their state in.
type libraryStore interface {
	librarystore.LibraryRootStore
	librarystore.LibraryFileStore
	librarystore.LibraryItemStore
	store.LibraryProjection
	librarystore.LibraryCatalogStore
	librarystore.LibraryNeedsStore
	librarystore.LibraryEpisodeOrderStore
	librarystore.LibraryCollectionStore
	librarystore.LibraryPeopleStore
	librarystore.LibraryItemFactsReader
	librarystore.LibraryTitleReader
	store.SettingStore
	inventory.Repository
	artRefs
}

// libraryBuild is what Settings → Library needs beyond the store and settings.
type libraryBuild struct {
	lib *library.Client
	// watched reports whether a viewer is fetching a channel; header reads wait while one is. Nil
	// without internal playout, when nothing on this box needs the disks first.
	watched func() bool
	// ffmpegPath locates ffprobe, the header reader's fallback.
	ffmpegPath string
	// images keeps the titles' posters; nil keeps none.
	images *images.Service
	// processes lists each ffprobe run in Diagnostics; nil lists none.
	processes *diagnostics.ProcessManager
	// hookSecret is the password Sonarr and Radarr send with each webhook.
	hookSecret string
	// notify tells the operators about offline folders and delayed changes; nil tells no one.
	notify libraryNotifier
	// tmdb is the catalog the titles' details come from; nil fetches none.
	tmdb *tmdb.Client
	// frames publishes the import's progress and title changes live; nil publishes none.
	frames libraryFrames
	log    *slog.Logger
	owner  *generationLifecycle
}

// libraryNotifier is productNotificationCoordinator's library half (#2151 decision 32).
type libraryNotifier interface {
	LibraryFolder(ctx context.Context, root string, readable bool)
	LibraryChangesDegraded(ctx context.Context, app string, err error)
	LibraryNeedsYou(ctx context.Context, total int)
}

// libraryNotifierOf is c as a libraryNotifier, nil (not a typed nil) when there is none.
func libraryNotifierOf(c *productNotificationCoordinator) libraryNotifier {
	if c == nil {
		return nil
	}
	return c
}

// libraryServices is Settings → Library and the arrs' webhooks; both nil without a store.
type libraryServices struct {
	roots libraryapi.LibraryRootService
	hooks libraryapi.LibraryHookService
}

// buildLibrary wires Settings → Library (#2151 decision 6) and starts the scanner, its folder
// watcher and the arr webhook registrations for the generation's lifetime. Sources, the path map,
// the arr connections, Loomarr's address and the kill switch are read per call, so a saved
// setting applies at once.
func buildLibrary(st libraryStore, set resolved, deps libraryBuild) libraryServices {
	if st == nil {
		return libraryServices{}
	}
	// The Library's rows follow every inventory write from here on (#2151 decision 43).
	st.ProjectLibraryWith(scan.ProjectLibraryItem)
	pathMap := func() library.PathMap { return library.ParsePathMap(settings.LibraryPathMap.Get(set)) }
	roots := &scan.Roots{
		Store:   st,
		Sources: func() []scan.Source { return libraryRootSources(set, deps.lib) },
		PathMap: pathMap,
		Disk:    scan.OSDisk{},
		Now:     time.Now,
	}
	inv := inventory.New(st)
	resolver, err := precedence.New("US")
	if err != nil {
		deps.log.Error("library scan: not started", "err", err)
		return libraryServices{roots: &scan.Library{Roots: roots, ItemStore: st, Inventory: inv, CollectionStore: st,
			Records: st}}
	}
	conns := set.arrConns()
	sonarr, radarr := arr.NewSonarr(conns.Sonarr, nil), arr.NewRadarr(conns.Radarr, nil)
	address := func(conn func() (string, string)) func() string {
		return func() string { u, _ := conn(); return u }
	}
	managers := []scan.Manager{
		{Authority: "sonarr", Lister: sonarr, Fetcher: sonarr, Address: address(conns.Sonarr)},
		{Authority: "radarr", Lister: radarr, Fetcher: radarr, Address: address(conns.Radarr)},
	}
	// The catalog the Library's details come from; its declared ids are the Library's catalog rows.
	var provider *mptmdb.Provider
	var keys scan.CatalogKeys
	if deps.tmdb != nil {
		provider = mptmdb.New(deps.tmdb, metadataLocale(set, deps.lib))
		keys = provider.Capabilities().Resolves
	}
	// One Tracker records the whole import: the walk and header reads here, the lookups below.
	tracker, titlesChanged := libraryFramesFor(deps.frames)
	im := &scan.Importer{
		Tracker:       tracker,
		TitlesChanged: titlesChanged,
		Store:         st,
		Inventory:     inv,
		Disk:          scan.OSDisk{},
		Managers:      func() []scan.Manager { return managers },
		Discover: func(ctx context.Context) error {
			_, _, err := roots.Discover(ctx)
			return err
		},
		NFO:      nfo.New(),
		Art:      localart.New(),
		Artwork:  libraryArtworkOf(deps.images, st),
		Catalog:  st,
		Keys:     keys,
		Answers:  st,
		Resolver: resolver,
		Header:   libraryHeader(playout.FFprobeSourceNextTo(deps.ffmpegPath, deps.processes)),
		Enabled:  func() bool { return settings.LibraryScannerEnabled.Get(set) },
		Now:      time.Now,
		Log:      deps.log,
	}
	im.Busy = deps.watched
	if deps.notify != nil {
		im.RootReadability = deps.notify.LibraryFolder
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
	if deps.notify != nil {
		hooks.Degraded = deps.notify.LibraryChangesDegraded
		notice := &scan.NeedsNotice{
			Store: st, Notify: deps.notify.LibraryNeedsYou, Enabled: im.Enabled, Log: deps.log,
			Settled: im.Settled,
		}
		deps.owner.goRunAfterBuild(notice.Run)
	}
	deps.owner.goRunAfterBuild(scanner.Run)
	deps.owner.goRunAfterBuild(watcher.Run)
	deps.owner.goRunAfterBuild(hooks.Run)
	lib := &scan.Library{Roots: roots, Scanner: scanner, ItemStore: st, Inventory: inv, NeedStore: st,
		Orders: st, CollectionStore: st, Records: st, Keys: keys}
	if provider != nil {
		enricher := &scan.Enricher{
			Store: st, Provider: provider, Rewrite: scanner.PassFolders,
			Language: func() string {
				language, err := provider.Language()
				if err != nil {
					return ""
				}
				return language
			},
			Enabled: im.Enabled, Log: deps.log, Tracker: tracker,
		}
		deps.owner.goRunAfterBuild(enricher.Run)
		lib.Enricher = enricher
		filmographer := &scan.Filmographer{Store: st, Catalog: provider, Enabled: im.Enabled, Log: deps.log}
		deps.owner.goRunAfterBuild(filmographer.Run)
		lib.Search = tmdbSearcher(deps.tmdb)
		matcher := &scan.Matcher{
			Store: st, Search: lib.Search, Provider: provider, Inventory: inv,
			Enabled: im.Enabled, Log: deps.log, Match: lib.MatchBySearch, Episodes: lib.EpisodesOnDisk,
		}
		deps.owner.goRunAfterBuild(matcher.Run)
	}
	return libraryServices{roots: lib, hooks: hooks}
}

// tmdbSearcher is TMDB's title search, as Needs you searches it.
func tmdbSearcher(client *tmdb.Client) scan.Searcher {
	return func(ctx context.Context, term string, limit int) ([]scan.SearchHit, error) {
		found, err := client.Search(ctx, term, limit)
		if err != nil {
			return nil, err
		}
		hits := make([]scan.SearchHit, 0, len(found))
		for _, c := range found {
			if c.TMDBID <= 0 {
				continue
			}
			hits = append(hits, scan.SearchHit{
				Kind: string(c.MediaType), ID: mp.ExternalID{Namespace: mp.NamespaceTMDB, Value: strconv.Itoa(c.TMDBID)}, Name: c.Name, Year: c.Year, Overview: c.Overview,
			})
		}
		return hits, nil
	}
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
		facts := playoutresolve.InventoryFactsOf(observed)
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
