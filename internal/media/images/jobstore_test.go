package images

import (
	"context"
	"slices"
	"time"
)

// The job half of the in-memory store, extending fakeStore so one double satisfies FetchStore,
// AVIFStore and GCStore.
//
// ⚠ Every list here re-implements the SQL's selection rule in Go, which is a duplication worth
// being uneasy about — a fake that disagrees with the real query makes a job look correct against
// a work list it will never actually be handed. The mitigation is that the store conformance suite
// runs the real queries against BOTH dialects, so these predicates are checked in one place and
// exercised in another rather than being the only statement of the rule.

func (f *fakeStore) ListAwaitingFetch(_ context.Context, limit int) ([]Image, error) {
	return f.pick(limit, func(img Image) bool {
		return img.Origin == OriginRemote && img.OriginFetchedAt.IsZero()
	}), nil
}

func (f *fakeStore) ListByOrigin(_ context.Context, origin Origin, limit int) ([]Image, error) {
	return f.pick(limit, func(img Image) bool { return img.Origin == origin }), nil
}

func (f *fakeStore) ListExpiredBefore(_ context.Context, cutoff time.Time, limit int) ([]Image, error) {
	return f.pick(limit, func(img Image) bool {
		// The `> 0` half matters: a never-fetched row must not look infinitely stale, or the
		// expiry sweep would purge the fetch job's entire backlog.
		return img.Origin == OriginRemote && !img.OriginFetchedAt.IsZero() && img.OriginFetchedAt.Before(cutoff)
	}), nil
}

func (f *fakeStore) ListOrphans(_ context.Context, limit int) ([]Image, error) {
	return f.pick(limit, func(img Image) bool {
		return !slices.ContainsFunc(f.refs, func(r Ref) bool { return r.ImageHash == img.Hash })
	}), nil
}

func (f *fakeStore) ListUnrecoverable(_ context.Context, limit int) ([]Image, error) {
	return f.pick(limit, func(img Image) bool { return !img.Origin.Recoverable() }), nil
}

func (f *fakeStore) ListMissingFormat(_ context.Context, recipe string, format Format, limit int) ([]Image, error) {
	return f.pick(limit, func(img Image) bool {
		ds := f.derivatives[img.Hash]
		if len(ds) == 0 {
			return false // no rendition at all: the fetch job's business, not the encoder's
		}
		return !slices.ContainsFunc(ds, func(d Derivative) bool { return d.Recipe == recipe && d.Format == format })
	}), nil
}

func (f *fakeStore) ListColdestDerivatives(_ context.Context, limit int) ([]Derivative, error) {
	var all []Derivative
	for _, ds := range f.derivatives {
		all = append(all, ds...)
	}
	// Ordered by the PARENT image's last use, which is the policy under test.
	slices.SortFunc(all, func(a, b Derivative) int {
		at, bt := f.images[a.ImageHash].LastUsedAt, f.images[b.ImageHash].LastUsedAt
		switch {
		case at.Before(bt):
			return -1
		case bt.Before(at):
			return 1
		}
		return 0
	})
	if limit > 0 && len(all) > limit {
		all = all[:limit]
	}
	return all, nil
}

func (f *fakeStore) TotalDerivativeBytes(context.Context) (int64, error) {
	var total int64
	for _, ds := range f.derivatives {
		for _, d := range ds {
			total += d.Bytes
		}
	}
	return total, nil
}

func (f *fakeStore) DeleteDerivative(_ context.Context, hash, recipe string, format Format, width int) error {
	f.derivatives[hash] = slices.DeleteFunc(f.derivatives[hash], func(d Derivative) bool {
		return d.Recipe == recipe && d.Format == format && d.Width == width
	})
	return nil
}

func (f *fakeStore) DeleteDerivatives(_ context.Context, hash string) error {
	delete(f.derivatives, hash)
	return nil
}

// DeleteImage mirrors the schema's ON DELETE CASCADE on both child tables. ⚠ Modelling the cascade
// is what makes the fetch job's ordering testable at all: fetchOne moves refs BEFORE deleting the
// old row precisely because the delete takes the refs with it, and a fake that left them behind
// would pass whichever order the code used.
func (f *fakeStore) DeleteImage(_ context.Context, hash string) error {
	delete(f.images, hash)
	delete(f.derivatives, hash)
	f.refs = slices.DeleteFunc(f.refs, func(r Ref) bool { return r.ImageHash == hash })
	return nil
}

// RepointRefs mirrors the store's insert-then-delete, including the collision no-op.
func (f *fakeStore) RepointRefs(_ context.Context, from, to string) error {
	for _, r := range f.refs {
		if r.ImageHash != from {
			continue
		}
		moved := Ref{ImageHash: to, OwnerKind: r.OwnerKind, OwnerID: r.OwnerID, Role: r.Role}
		if !slices.Contains(f.refs, moved) {
			f.refs = append(f.refs, moved)
		}
	}
	f.refs = slices.DeleteFunc(f.refs, func(r Ref) bool { return r.ImageHash == from })
	return nil
}

// pick applies a predicate in a STABLE order.
//
// ⚠ Sorted by hash rather than ranged over the map directly. A Go map range is randomised, and a
// job test asserting "the coldest one went first" over a randomised work list is the flaky-by-
// construction shape this repo already has one open bug about (TestConfirm_WritesReviewedSegments).
func (f *fakeStore) pick(limit int, keep func(Image) bool) []Image {
	hashes := make([]string, 0, len(f.images))
	for h := range f.images {
		hashes = append(hashes, h)
	}
	slices.Sort(hashes)

	out := make([]Image, 0, len(hashes))
	for _, h := range hashes {
		if img := f.images[h]; keep(img) {
			out = append(out, img)
			if limit > 0 && len(out) >= limit {
				break
			}
		}
	}
	return out
}

// refsFor is a test-side lookup the production contract deliberately does not offer.
func (f *fakeStore) refsFor(hash string) []Ref {
	var out []Ref
	for _, r := range f.refs {
		if r.ImageHash == hash {
			out = append(out, r)
		}
	}
	return out
}
