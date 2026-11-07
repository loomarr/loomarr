package app

import (
	"context"
	"errors"
	"time"

	"github.com/loomarr/loomarr/internal/api"
	"github.com/loomarr/loomarr/internal/library"
	"github.com/loomarr/loomarr/internal/library/scan"
	"github.com/loomarr/loomarr/internal/requester"
	"github.com/loomarr/loomarr/internal/settings"
	"github.com/loomarr/loomarr/internal/store"
)

// buildLibraryRoots wires Settings → Library's media folders (#2151 decision 6). Sources, the
// path map and the arr connections are read per call, so a saved connection applies at once.
func buildLibraryRoots(st store.LibraryRootStore, set resolved, lib *library.Client) api.LibraryRootService {
	if st == nil {
		return nil
	}
	return &scan.Library{Roots: &scan.Roots{
		Store:   st,
		Sources: func() []scan.Source { return libraryRootSources(set, lib) },
		PathMap: func() library.PathMap { return library.ParsePathMap(settings.LibraryPathMap.Get(set)) },
		Disk:    scan.OSDisk{},
		Now:     time.Now,
	}}
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
	arr := requester.NewArr(set.arrConns())
	for _, app := range []string{"sonarr", "radarr"} {
		sources = append(sources, scan.Source{Authority: app, Folders: func(ctx context.Context) ([]string, error) {
			folders, err := arr.RootFolders(ctx, app)
			if errors.Is(err, requester.ErrArrNotConfigured) {
				return nil, scan.ErrSkipped
			}
			return folders, err
		}})
	}
	return sources
}
