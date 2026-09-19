package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/api"
	"github.com/loomarr/loomarr/internal/filler"
)

func TestFillerExactClipSourceProvenance(t *testing.T) {
	harness := newFillerHarness(t)
	srv, st := harness.Server, harness.Store
	ctx := context.Background()
	now := time.Now().UTC()
	putClip(t, st, filler.Clip{Hash: "source-reel", Path: "reel.mp4", Name: "Source recording", Kind: filler.Commercial, IsComposite: true})
	putClip(t, st, filler.Clip{Hash: "source-child", Path: "child.mp4", Name: "Rendered child", Kind: filler.Commercial, ParentHash: "source-reel"})
	putClip(t, st, filler.Clip{Hash: "unrelated", Path: "reel.mp4", Name: "Reused filename", Kind: filler.Commercial})
	if err := st.UpsertAcquisitionRun(ctx, filler.AcquisitionRun{ID: "source-run", Trigger: filler.AcquisitionManual, Status: filler.AcquisitionSuccess, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	artifact := filler.AcquisitionArtifact{
		ID: "source-artifact", AcquisitionID: "source-run", Provider: "archive", SourceID: "classic",
		SourceURL: "https://archive.org/details/original-reel", RemoteID: "original-reel",
		MediaPath: "reel.mp4", ClipHash: "source-reel", MediaSHA256: strings.Repeat("a", 64),
		MediaBytes: 42, State: filler.ArtifactConsumed, CompletedAt: now, UpdatedAt: now,
	}
	if err := st.UpsertAcquisitionArtifacts(ctx, []filler.AcquisitionArtifact{artifact}); err != nil {
		t.Fatal(err)
	}
	read := func(query string) []api.ClipDTO {
		t.Helper()
		res := do(t, srv, http.MethodGet, "/v1/filler?includeComposites=true&"+query, memberToken, "")
		defer func() { _ = res.Body.Close() }()
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status = %d", res.StatusCode)
		}
		var body struct {
			Clips []api.ClipDTO `json:"clips"`
		}
		if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		return body.Clips
	}
	for _, hash := range []string{"source-reel", "source-child"} {
		rows := read("hashes=" + hash)
		if len(rows) != 1 || rows[0].SourceURL != artifact.SourceURL {
			t.Fatalf("%s provenance = %+v", hash, rows)
		}
	}
	for _, query := range []string{"hashes=unrelated", "limit=60", "hashes=source-reel&hashes=source-child"} {
		for _, row := range read(query) {
			if row.SourceURL != "" {
				t.Fatalf("unexpected provenance for %s: %+v", query, row)
			}
		}
	}
	for _, unsafe := range []string{"javascript:alert(1)", "https://secret:password@archive.org/details/reel", "file:///private/reel.mp4"} {
		artifact.SourceURL = unsafe
		if err := st.UpsertAcquisitionArtifacts(ctx, []filler.AcquisitionArtifact{artifact}); err != nil {
			t.Fatal(err)
		}
		if rows := read("hashes=source-reel"); len(rows) != 1 || rows[0].SourceURL != "" {
			t.Fatalf("unsafe source exposed: %+v", rows)
		}
	}
}
