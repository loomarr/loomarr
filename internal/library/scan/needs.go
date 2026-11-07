package scan

import (
	"context"
	"errors"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/inventory"
	"github.com/loomarr/loomarr/internal/store"
	"github.com/loomarr/loomarr/internal/tmdb"
)

// ErrNotAnswerable is a title the maintainer cannot answer for: one the scanner did not write, so
// it has no scan origin to keep the answer against.
var ErrNotAnswerable = errors.New("this title has no scan origin")

// Need is a title that needs the maintainer, with what a search found for it, best first.
type Need struct {
	store.LibraryNeed
	Candidates []store.LibraryCandidate
	// Present is its present files (a series' episodes), in path order.
	Present []store.LibraryFile
}

// Needs lists Needs you: every present title nothing grounds and nobody answered.
func (l *Library) Needs(ctx context.Context) ([]Need, error) {
	needs, err := l.NeedStore.LibraryNeeds(ctx)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(needs))
	for _, n := range needs {
		ids = append(ids, n.ID)
	}
	candidates, err := l.NeedStore.LibraryCandidates(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]Need, 0, len(needs))
	for _, n := range needs {
		files, err := l.ItemStore.LibraryItemFiles(ctx, n.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, Need{LibraryNeed: n, Candidates: candidates[n.ID], Present: files})
	}
	return out, nil
}

// NeedCount is how many titles need the maintainer: the Needs you heading and the nav badge.
func (l *Library) NeedCount(ctx context.Context) (int, error) {
	needs, err := l.NeedStore.LibraryNeeds(ctx)
	return len(needs), err
}

// Link answers a title: it is the TMDB title tmdbID. Its files are written again with the link.
func (l *Library) Link(ctx context.Context, itemID, tmdbID string) error {
	tmdbID = strings.TrimSpace(tmdbID)
	if tmdbID == "" {
		return errors.New("a TMDB id is required")
	}
	return l.answer(ctx, itemID, store.LibraryAnswer{TMDBID: tmdbID})
}

// Ignore answers a title: it is not one. It leaves the Library and Needs you.
func (l *Library) Ignore(ctx context.Context, itemID string) error {
	return l.answer(ctx, itemID, store.LibraryAnswer{Ignored: true})
}

// Unanswer takes a title's answer back. A title nothing else grounds needs the maintainer again.
func (l *Library) Unanswer(ctx context.Context, itemID string) error {
	_, origin, err := l.origin(ctx, itemID)
	if err != nil {
		return err
	}
	if err := l.NeedStore.DeleteLibraryAnswer(ctx, origin); err != nil {
		return err
	}
	return l.rewrite(ctx, itemID, origin)
}

// SearchTitles searches the catalog for a title by name, for "Pick a different title". kind ""
// is movies and series.
func (l *Library) SearchTitles(ctx context.Context, term, kind string) ([]SearchHit, error) {
	if l.Search == nil {
		return nil, ErrNoCatalog
	}
	hits, err := l.Search(ctx, term, 20)
	if errors.Is(err, tmdb.ErrAPIKeyRequired) {
		return nil, ErrNoCatalog
	}
	if err != nil {
		return nil, err
	}
	out := hits[:0]
	for _, h := range hits {
		if kind == "" || h.Kind == kind {
			out = append(out, h)
		}
	}
	return out, nil
}

// ErrNoCatalog is a search with no catalog, or one without its key.
var ErrNoCatalog = errors.New("no catalog to search")

func (l *Library) answer(ctx context.Context, itemID string, a store.LibraryAnswer) error {
	kind, origin, err := l.origin(ctx, itemID)
	if err != nil {
		return err
	}
	a.Origin, a.Kind, a.AnsweredAt = origin, kind, l.now()
	if err := l.NeedStore.SaveLibraryAnswer(ctx, a); err != nil {
		return err
	}
	return l.rewrite(ctx, itemID, origin)
}

func (l *Library) origin(ctx context.Context, itemID string) (string, store.LibraryOrigin, error) {
	kind, origin, err := l.NeedStore.LibraryItemOrigin(ctx, itemID)
	if err != nil {
		return "", store.LibraryOrigin{}, err
	}
	if origin.Key == "" || (kind != "movie" && kind != "series") {
		return "", store.LibraryOrigin{}, ErrNotAnswerable
	}
	return kind, origin, nil
}

// rewrite sends a title's folders to the scanner, which writes its files again. A link can make
// a title's file a copy of another title, which then holds the origin, so that title's files are
// written again too: taking the link back has to find the file where it went.
func (l *Library) rewrite(ctx context.Context, itemID string, origin store.LibraryOrigin) error {
	ids := []string{itemID}
	if l.Inventory != nil {
		key := inventory.OriginKey{Authority: inventory.AuthorityID(origin.Authority), ExternalItemID: origin.Key}
		holder, ok, err := l.Inventory.Item(ctx, inventory.ItemRef{Origin: &key})
		if err != nil {
			return err
		}
		if ok && string(holder.ID) != itemID {
			ids = append(ids, string(holder.ID))
		}
	}
	var files []store.LibraryFilePath
	for _, id := range ids {
		found, err := l.NeedStore.RewriteLibraryItemFiles(ctx, id)
		if err != nil {
			return err
		}
		files = append(files, found...)
	}
	l.pass(files)
	return nil
}

func (l *Library) now() time.Time {
	if l.Now != nil {
		return l.Now()
	}
	return time.Now()
}

// rewriteFolders asks for a pass over the folders holding files, one call per root.
func rewriteFolders(files []store.LibraryFilePath, rewrite func(root string, dirs ...string)) {
	dirs := map[string]map[string]bool{}
	for _, f := range files {
		if dirs[f.Root] == nil {
			dirs[f.Root] = map[string]bool{}
		}
		dirs[f.Root][filepath.Dir(f.Path)] = true
	}
	for root, set := range dirs {
		list := make([]string, 0, len(set))
		for d := range set {
			list = append(list, d)
		}
		sort.Strings(list)
		rewrite(root, list...)
	}
}
