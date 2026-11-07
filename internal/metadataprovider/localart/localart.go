// Package localart provides the artwork beside the media as a metadata provider (#2151 decision
// 19). Kodi's naming convention, which Emby, Jellyfin and the arrs write, puts a movie's poster in
// "<name>-poster.jpg" or the folder's "poster.jpg", an episode's still in "<name>-thumb.jpg", and a
// series' art in its folder.
//
// A record carries references (the image's path), never bytes: the image service imports a poster
// when the scanner writes its title, keyed by its content. Only regular files are referenced, so a
// symlink never points the import outside the share; the importer checks again when it opens one.
package localart

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	mp "github.com/loomarr/loomarr/internal/metadataprovider"
)

// Artwork kinds, named as the arrs and TMDB name them so precedence picks one per kind.
const (
	KindPoster    = "poster"
	KindFanart    = "fanart"
	KindClearlogo = "clearlogo"
	KindBanner    = "banner"
	KindLandscape = "landscape"
	KindClearart  = "clearart"
	KindDiscart   = "discart"
	// KindThumb is an episode's still.
	KindThumb = "thumb"
)

// extensions are the image types a sidecar may use, in the order one is preferred.
var extensions = []string{".jpg", ".jpeg", ".png", ".webp"}

// IsImage reports whether name has an extension artwork may use, whatever its case.
func IsImage(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	for _, e := range extensions {
		if ext == e {
			return true
		}
	}
	return false
}

// rule is one artwork kind and the names, without extension, that hold it, best first.
type rule struct {
	kind  string
	names []string
}

// folderRules are the names a series folder, or a folder holding one movie, uses.
var folderRules = []rule{
	{KindPoster, []string{"poster", "folder", "cover"}},
	{KindFanart, []string{"fanart", "backdrop", "background"}},
	{KindClearlogo, []string{"clearlogo", "logo"}},
	{KindBanner, []string{"banner"}},
	{KindLandscape, []string{"landscape"}},
	{KindClearart, []string{"clearart"}},
	{KindDiscart, []string{"discart", "disc"}},
}

// movieKinds are the kinds a movie file's own "<name>-<kind>" sidecar may hold.
var movieKinds = []string{KindPoster, KindFanart, KindClearlogo, KindBanner, KindLandscape, KindClearart, KindDiscart}

// Provider reads artwork sidecars from the local filesystem.
type Provider struct{}

// New returns the local artwork provider.
func New() *Provider { return &Provider{} }

func (*Provider) ID() mp.ID { return mp.IDLocalArt }

// Capabilities: artwork is a sidecar, resolved by the path of the media it sits beside. Finding
// it is one directory listing, paced like the scanner's other cold reads.
func (*Provider) Capabilities() mp.Capabilities {
	return mp.Capabilities{
		Basis: mp.BasisSidecar,
		Kinds: []mp.Kind{mp.KindMovie, mp.KindSeries, mp.KindEpisode},
		Resolves: []mp.Resolvable{
			{Kind: mp.KindMovie, Namespace: mp.NamespaceFile},
			{Kind: mp.KindSeries, Namespace: mp.NamespaceFile},
			{Kind: mp.KindEpisode, Namespace: mp.NamespaceFile},
		},
		Families: []mp.Family{mp.FamilyArtwork},
		Rate:     mp.RateHint{MaxConcurrent: 4},
	}
}

// Fetch lists the artwork for ref. For a movie or episode, ref names the media file; for a series,
// it names the series folder. A movie's own "<name>-poster.jpg" wins over the folder's
// "poster.jpg", which counts only when the movie is the folder's sole video. No artwork is
// ErrNotFound.
func (p *Provider) Fetch(ctx context.Context, ref mp.Ref) (mp.Record, error) {
	if !p.Capabilities().CanResolve(ref) {
		return mp.Record{}, fmt.Errorf("localart: %s by %s: %w", ref.Kind, ref.ID.Namespace, mp.ErrUnsupported)
	}
	if err := ctx.Err(); err != nil {
		return mp.Record{}, err
	}
	media := filepath.Clean(ref.ID.Value)
	if !filepath.IsAbs(media) {
		return mp.Record{}, fmt.Errorf("localart: path %q is not absolute: %w", ref.ID.Value, mp.ErrUnsupported)
	}
	dir := filepath.Dir(media)
	if ref.Kind == mp.KindSeries {
		dir = media
	}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return mp.Record{}, fmt.Errorf("localart: no folder for %s: %w", ref.Kind, mp.ErrNotFound)
	}
	if err != nil {
		return mp.Record{}, err
	}
	var names []string
	videos := 0
	for _, e := range entries {
		if !e.Type().IsRegular() {
			continue
		}
		names = append(names, e.Name())
		if mp.IsVideoFile(e.Name()) {
			videos++
		}
	}
	return Beside(ref.Kind, media, names, videos <= 1)
}

// Beside picks the artwork for media from names, the regular files in its folder, without reading
// the folder: a scanner that has just listed it passes what it saw. For a series, media is the
// folder. sole is whether a movie is its folder's only video, the condition for the folder's own
// art to be the movie's. No artwork is ErrNotFound.
func Beside(kind mp.Kind, media string, names []string, sole bool) (mp.Record, error) {
	media = filepath.Clean(media)
	dir := filepath.Dir(media)
	if kind == mp.KindSeries {
		dir = media
	}
	files := make(map[string]string, len(names))
	for _, name := range names {
		files[strings.ToLower(filepath.Base(name))] = filepath.Base(name)
	}

	stem := strings.TrimSuffix(filepath.Base(media), filepath.Ext(media))
	var rules []rule
	switch kind {
	case mp.KindEpisode:
		rules = []rule{{KindThumb, []string{stem + "-thumb"}}}
	case mp.KindMovie:
		for _, kind := range movieKinds {
			rules = append(rules, rule{kind, []string{stem + "-" + kind}})
		}
		if sole {
			rules = append(rules, folderRules...)
		}
	case mp.KindSeries:
		rules = folderRules
	}

	var art []mp.ArtworkRef
	have := map[string]bool{}
	for _, r := range rules {
		if have[r.kind] {
			continue
		}
		if name, ok := find(files, r.names); ok {
			art = append(art, mp.ArtworkRef{Kind: r.kind, Ref: filepath.Join(dir, name)})
			have[r.kind] = true
		}
	}
	if len(art) == 0 {
		return mp.Record{}, fmt.Errorf("localart: no artwork for %s: %w", kind, mp.ErrNotFound)
	}
	return mp.Record{Source: mp.Source{Provider: mp.IDLocalArt, Basis: mp.BasisSidecar}, Kind: kind, Artwork: art}, nil
}

// find returns the first of names present with an image extension, as the folder spells it.
// Matching ignores case: a share's names are as their writer typed them.
func find(files map[string]string, names []string) (string, bool) {
	for _, name := range names {
		for _, ext := range extensions {
			if actual, ok := files[strings.ToLower(name+ext)]; ok {
				return actual, true
			}
		}
	}
	return "", false
}
