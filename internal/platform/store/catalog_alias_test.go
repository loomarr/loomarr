package store

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/filler/clips"
	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
	"github.com/loomarr/loomarr/internal/platform/store/storedb"
)

// The store re-exports the catalog cluster's sentinels as the same values. errors.Is compares by
// identity, so a copy that merely prints the same text would make every caller matching the other
// name silently false.
func TestCatalogSentinelsAreTheCatalogstoreValues(t *testing.T) {
	for _, c := range []struct {
		name         string
		store, inner error
		message      string
	}{
		{"ErrConditionedClipMismatch", ErrConditionedClipMismatch, clipstore.ErrConditionedClipMismatch, "store: conditioned clip does not match the catalog"},
		{"ErrTaxonConflict", ErrTaxonConflict, clipstore.ErrTaxonConflict, "store: taxonomy conflict"},
		{"ErrCorruptClipDefects", ErrCorruptClipDefects, clipstore.ErrCorruptClipDefects, "corrupt clip defect records"},
		{"ErrInferiorCopyMismatch", ErrInferiorCopyMismatch, clipstore.ErrInferiorCopyMismatch, "store: inferior copy must name two different existing clips"},
		{"ErrUnknownClipSort", ErrUnknownClipSort, clipstore.ErrUnknownClipSort, "unknown clip sort"},
		{"ErrNotFound", ErrNotFound, clipstore.ErrNotFound, "store: not found"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if !errors.Is(c.store, c.inner) || !errors.Is(c.inner, c.store) {
				t.Fatalf("store.%s and clipstore.%s are not the same value", c.name, c.name)
			}
			if !errors.Is(fmt.Errorf("wrapped: %w", c.inner), c.store) {
				t.Fatalf("errors.Is(wrapped clipstore error, store.%s) = false", c.name)
			}
			if got := c.store.Error(); got != c.message {
				t.Fatalf("message = %q, want %q (a log string must not change)", got, c.message)
			}
		})
	}
	if !errors.Is(clipstore.ErrNotFound, storedb.ErrNotFound) {
		t.Fatal("clipstore.ErrNotFound is not the storedb.ErrNotFound value")
	}
}

// Real read paths: each sentinel is produced inside clipstore and must match the store's name.
func TestCatalogErrorsMatchTheStoreSentinels(t *testing.T) {
	ctx := context.Background()
	st := newSQLiteStore(t)
	if _, err := st.GetClip(ctx, "nobody"); !errors.Is(err, ErrNotFound) || !errors.Is(err, clipstore.ErrNotFound) {
		t.Fatalf("GetClip miss = %v, want store.ErrNotFound and clipstore.ErrNotFound", err)
	}
	if _, err := st.ListClips(ctx, ClipFilter{Sort: "bogus"}); !errors.Is(err, ErrUnknownClipSort) || !errors.Is(err, clipstore.ErrUnknownClipSort) {
		t.Fatalf("ListClips with an unknown sort = %v, want store.ErrUnknownClipSort", err)
	}
	if _, err := st.PreviewTaxonomyEdit(ctx, TaxonomyEdit{Delete: true, Slug: "halloween"}); !errors.Is(err, ErrTaxonConflict) || !errors.Is(err, clipstore.ErrTaxonConflict) {
		t.Fatalf("deleting a dated holiday's taxon = %v, want store.ErrTaxonConflict", err)
	}
	if err := st.MergeInferiorCopy(ctx, InferiorCopy{}); !errors.Is(err, ErrInferiorCopyMismatch) {
		t.Fatalf("MergeInferiorCopy with no hashes = %v, want store.ErrInferiorCopyMismatch", err)
	}
}

// Types are aliases: values built under either name need no conversion, and the aggregate store
// returns the cluster's own types.
var (
	_ clipstore.Clip               = Clip{}
	_ clipstore.ClipFilter         = ClipFilter{}
	_ clipstore.ClipSort           = ClipSortName
	_ ClipSort                     = clipstore.ClipSortPlays
	_ clipstore.ClipTx             = ClipTx{}
	_ clipstore.ClipGeography      = ClipGeography{}
	_ clipstore.ClipPublication    = ClipPublication{}
	_ clipstore.ClipArtworkPending = ClipArtworkPending{}
	_ clipstore.WantedCopy         = WantedCopy{}
	_ clipstore.InferiorCopy       = InferiorCopy{}
	_ clipstore.ClipDefectRecord   = ClipDefectRecord{}
	_ clipstore.ClipDefectLabel    = ClipDefectLabel{}
	_ clipstore.StoreTaxon         = StoreTaxon{}
	_ clipstore.TaxonomyEdit       = TaxonomyEdit{}
	_ clipstore.TaxonomyEditImpact = TaxonomyEditImpact{}
	_ clipstore.TaxonUsage         = TaxonUsage{}
	_ clipstore.TaxonomyUsage      = TaxonomyUsage{}
	_ clipstore.Image              = Image{}
	_ clipstore.ImageRef           = ImageRef{}
	_ clipstore.ImageDerivative    = ImageDerivative{}
)

// The embedded cluster is what satisfies the image role and the catalog methods: the aggregate store
// hands out the same *clipstore.Store methods, with no forwarding layer in between.
func TestSQLStoreCatalogIsTheEmbeddedCluster(t *testing.T) {
	s, ok := adapterOf(newSQLiteStore(t))
	if !ok || s.catalogCluster == nil || s.identityCluster == nil {
		t.Fatal("sqlStore has a nil cluster: a constructor skipped bindClusters")
	}
	var _ ImageStore = s.catalogCluster
}

// store.Handle.Clips is the extension stores' way to a ClipTx; it must hand back the catalog's own
// type, bound to the same cluster, so fillerstore's writes keep one owner.
func TestHandleClipsReturnsTheCatalogClipTx(t *testing.T) {
	ctx := context.Background()
	st := newSQLiteStore(t)
	h := HandleOf(st)
	tx, err := h.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	clips := h.Clips(tx)
	func(clipstore.ClipTx) {}(clips) // compiles only while Handle.Clips returns the catalog's own type
	if err := clips.Require(ctx, "nobody"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("ClipTx.Require for a missing clip = %v, want store.ErrNotFound", err)
	}
}

// The catalog reaches channel policies only through clipstore.ChannelPolicies, implemented here.
// Rekeying a clip must carry a channel's pin and exclusion to the new hash and bump that channel's
// revision once, and must leave a channel that never named the clip untouched.
func TestChannelPoliciesHookRewritesAndBumpsRevision(t *testing.T) {
	ctx := context.Background()
	s := newSQLiteStore(t).(*sqlStore)
	now := time.Unix(1_700_000_000, 0).UTC()

	pinning := sampleChannel("pins", 801, now)
	pinning.Policy.Filler = &schedule.FillerSelection{Pinned: []string{"keep", "old"}, Excluded: []string{"old", "other"}}
	pinning, err := s.SaveChannel(ctx, pinning)
	if err != nil {
		t.Fatal(err)
	}
	bystander := sampleChannel("bystander", 802, now)
	bystander.Policy.Filler = &schedule.FillerSelection{Pinned: []string{"keep"}}
	bystander, err = s.SaveChannel(ctx, bystander)
	if err != nil {
		t.Fatal(err)
	}

	clip := Clip{UpdatedAt: now}
	clip.Hash, clip.Path, clip.Name, clip.Kind = "old", "ol/d0/old.mp4", "Old", clips.Commercial
	clip.DurationMs, clip.Source = 30_000, "archive"
	if err := s.UpsertClip(ctx, clip); err != nil {
		t.Fatal(err)
	}
	replacement := clip
	replacement.Hash, replacement.Path = "new", "ne/w0/new.mp4"
	if err := s.ReplaceClipIdentity(ctx, "old", replacement); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetChannel(ctx, pinning.ID)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"keep", "new"}; !slices.Equal(got.Policy.Filler.Pinned, want) {
		t.Errorf("pinned = %v, want %v", got.Policy.Filler.Pinned, want)
	}
	if want := []string{"new", "other"}; !slices.Equal(got.Policy.Filler.Excluded, want) {
		t.Errorf("excluded = %v, want %v", got.Policy.Filler.Excluded, want)
	}
	if got.Revision != pinning.Revision+1 {
		t.Errorf("revision = %d, want %d: the rewrite must bump it exactly once", got.Revision, pinning.Revision+1)
	}
	other, err := s.GetChannel(ctx, bystander.ID)
	if err != nil {
		t.Fatal(err)
	}
	if other.Revision != bystander.Revision {
		t.Errorf("a channel that pinned nothing was rewritten: revision %d, want %d", other.Revision, bystander.Revision)
	}
}
