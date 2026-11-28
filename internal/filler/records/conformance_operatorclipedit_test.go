package records

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler/clips"
	"github.com/loomarr/loomarr/internal/filler/taxonomy"
)

func testOperatorClipEdit(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	at := time.Unix(1_800_000_000, 0).UTC()
	clip := sampleClip("op1", "operator.mp4", clips.Commercial, 1990, clips.General, "")
	if err := s.UpsertClip(ctx, clip); err != nil {
		t.Fatal(err)
	}
	before, err := s.GetClip(ctx, clip.Hash)
	if err != nil {
		t.Fatal(err)
	}
	name, brand, role := "Renamed", "Acme", string(clips.Bumper)
	badEra, era := 1900, 1995
	unchanged := func(step string) {
		t.Helper()
		got, err := s.GetClip(ctx, clip.Hash)
		if err != nil {
			t.Fatal(err)
		}
		states, err := s.ListFillerEnrichment(ctx, clip.Hash)
		if err != nil {
			t.Fatal(err)
		}
		if got.Name != before.Name || got.Brand != before.Brand || got.Role != before.Role || got.Era != before.Era || len(states) != 0 {
			t.Fatalf("%s: clip = %q/%q/%q/%d with %d evidence rows, want it untouched", step, got.Name, got.Brand, got.Role, got.Era, len(states))
		}
	}

	// The era is refused as evidence only after every column has been written.
	err = s.ApplyOperatorClipEdit(ctx, OperatorClipEdit{
		Hash: clip.Hash, At: at, Reference: "test", Name: &name, Brand: &brand, Role: &role, Era: &badEra,
	})
	if err == nil {
		t.Fatal("an era outside the supported range was accepted")
	}
	unchanged("failed evidence")

	err = s.ApplyOperatorClipEdit(ctx, OperatorClipEdit{
		Hash: clip.Hash, At: at, Reference: "test", Name: &name,
		Tags: &OperatorTags{Leaves: []string{"no-such-taxon"}, Forest: &taxonomy.Forest{}},
	})
	if !errors.Is(err, ErrTaxonConflict) {
		t.Fatalf("unknown taxon = %v, want ErrTaxonConflict", err)
	}
	unchanged("taxon conflict")

	if err := s.ApplyOperatorClipEdit(ctx, OperatorClipEdit{Hash: "missing", At: at, Reference: "test", Name: &name}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing clip = %v, want ErrNotFound", err)
	}

	if err := s.ApplyOperatorClipEdit(ctx, OperatorClipEdit{
		Hash: clip.Hash, At: at, Reference: "test", Name: &name, Brand: &brand, Role: &role, Era: &era,
	}); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetClip(ctx, clip.Hash)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != name || got.Brand != brand || string(got.Role) != role || got.Era != era || got.Audience != before.Audience {
		t.Fatalf("clip = %q/%q/%q/%d/%q after the edit", got.Name, got.Brand, got.Role, got.Era, got.Audience)
	}
}
