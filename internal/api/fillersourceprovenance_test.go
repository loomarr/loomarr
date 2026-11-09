package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	fillerapi "github.com/loomarr/loomarr/internal/api/filler"
	"github.com/loomarr/loomarr/internal/filler/acquisition"
	"github.com/loomarr/loomarr/internal/filler/clips"
)

func TestFillerExactClipSourceProvenance(t *testing.T) {
	srv, st, _ := newFillerServer(t)
	ctx := context.Background()
	now := time.Now().UTC()
	putClip(t, st, clips.Clip{Hash: "source-reel", Path: "reel.mp4", Name: "Source recording", Role: clips.Commercial, IsComposite: true})
	putClip(t, st, clips.Clip{Hash: "source-child", Path: "child.mp4", Name: "Rendered child", Role: clips.Commercial, ParentHash: "source-reel"})
	putClip(t, st, clips.Clip{Hash: "unrelated", Path: "reel.mp4", Name: "Reused filename", Role: clips.Commercial})
	if err := st.UpsertAcquisitionRun(ctx, acquisition.AcquisitionRun{ID: "source-run", Trigger: acquisition.TriggerManual, Status: acquisition.RunSuccess, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	artifact := acquisition.Artifact{
		ID: "source-artifact", AcquisitionID: "source-run", Provider: "archive", SourceID: "classic",
		SourceURL: "https://archive.org/details/original-reel", RemoteID: "original-reel",
		MediaPath: "reel.mp4", ClipHash: "source-reel", MediaSHA256: strings.Repeat("a", 64),
		MediaBytes: 42, State: acquisition.ArtifactConsumed, CompletedAt: now, UpdatedAt: now,
	}
	if err := st.UpsertAcquisitionArtifacts(ctx, []acquisition.Artifact{artifact}); err != nil {
		t.Fatal(err)
	}
	read := func(query string) []fillerapi.ClipDTO {
		t.Helper()
		res := do(t, srv, http.MethodGet, "/v1/filler?includeComposites=true&"+query, memberToken, "")
		defer func() { _ = res.Body.Close() }()
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status = %d", res.StatusCode)
		}
		var body struct {
			Clips []fillerapi.ClipDTO `json:"clips"`
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
		if err := st.UpsertAcquisitionArtifacts(ctx, []acquisition.Artifact{artifact}); err != nil {
			t.Fatal(err)
		}
		if rows := read("hashes=source-reel"); len(rows) != 1 || rows[0].SourceURL != "" {
			t.Fatalf("unsafe source exposed: %+v", rows)
		}
	}
}
