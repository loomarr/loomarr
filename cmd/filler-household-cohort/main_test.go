package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/fillerrelease"
)

func TestRunBuildsExactCohortFromShippingPipelineEvidence(t *testing.T) {
	root := t.TempDir()
	databasePath := filepath.Join(root, "loomarr.db")
	db, err := sql.Open("sqlite", "file:"+databasePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
CREATE TABLE clips (hash TEXT PRIMARY KEY, path TEXT NOT NULL, source TEXT NOT NULL, parent_hash TEXT NOT NULL);
CREATE TABLE filler_clip_pipeline (clip_hash TEXT PRIMARY KEY, disposition TEXT NOT NULL);`); err != nil {
		t.Fatal(err)
	}
	fillerRoot := filepath.Join(root, "filler")
	var seed seedArtifact
	for index := range 50 {
		name := fmt.Sprintf("case-%02d", index)
		caseID := "archive.org/fixture/" + name
		seed.SelectedCaseIDs = append(seed.SelectedCaseIDs, caseID)
		clipHash := hashBytes([]byte("clip:" + name))
		relative := filepath.Join(clipHash[:2], clipHash[2:4], clipHash+".mp4")
		disposition := "ready"
		if index >= 37 {
			disposition = "rejected"
		}
		if _, err := db.Exec(`INSERT INTO clips(hash,path,source,parent_hash) VALUES(?,?,?,?)`, clipHash, relative, "filler-dir", ""); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO filler_clip_pipeline(clip_hash,disposition) VALUES(?,?)`, clipHash, disposition); err != nil {
			t.Fatal(err)
		}
		var sidecar clipSidecar
		sidecar.Loomarr.OriginalName = name + ".mp4"
		if disposition == "ready" {
			playbackBytes := []byte("playback:" + name)
			sourceBytes := []byte("source:" + name)
			sourceRel := filepath.Join("masters", name+".mp4")
			writeFixture(t, filepath.Join(fillerRoot, relative), playbackBytes)
			writeFixture(t, filepath.Join(fillerRoot, sourceRel), sourceBytes)
			sidecar.Loomarr.MediaAssets.SourceMaster = mediaAsset{Path: sourceRel, SHA256: hashBytes(sourceBytes)}
			sidecar.Loomarr.MediaAssets.Playback.Asset = mediaAsset{Path: relative, SHA256: hashBytes(playbackBytes)}
			sidecar.Loomarr.MediaAssets.Playback.QC.CompleteDecode = true
			sidecar.Loomarr.MediaAssets.Playback.QC.Seekable = true
			sidecar.Loomarr.MediaAssets.Playback.QC.FastStart = true
		}
		sidecarBytes, err := json.Marshal(sidecar)
		if err != nil {
			t.Fatal(err)
		}
		writeFixture(t, filepath.Join(fillerRoot, relative[:len(relative)-len(filepath.Ext(relative))]+".info.json"), sidecarBytes)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	seedBytes, _ := json.Marshal(seed)
	seedPath := filepath.Join(root, "seed.json")
	writeFixture(t, seedPath, seedBytes)
	familiesPath := filepath.Join(root, "families.json")
	writeFixture(t, familiesPath, []byte(`{"families":[]}`))
	cookiePath := filepath.Join(root, "cookies.txt")
	writeFixture(t, cookiePath, []byte("#HttpOnly_localhost\tFALSE\t/\tFALSE\t1999999999\tloomarr_session\tfixture-token\n"))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		cookie, cookieErr := request.Cookie("loomarr_session")
		if cookieErr != nil || cookie.Value != "fixture-token" || request.Header.Get("Range") != "bytes=0-1023" {
			http.Error(w, "bad probe", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Range", "bytes 0-3/4")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte("test"))
	}))
	defer server.Close()
	outPath := filepath.Join(root, "cohort.json")
	var stdout, stderr bytes.Buffer
	exit := run([]string{
		"-db", databasePath, "-filler-root", fillerRoot, "-seed", seedPath, "-families", familiesPath,
		"-base-url", server.URL, "-cookie-file", cookiePath, "-out", outPath,
	}, &stdout, &stderr, func() time.Time { return time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC) }, server.Client())
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr.String())
	}
	encoded, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	var artifact outputArtifact
	if err := json.Unmarshal(encoded, &artifact); err != nil {
		t.Fatal(err)
	}
	if len(artifact.Selection.Selected) != 32 || len(artifact.Selection.Reserves) != 5 || len(artifact.Selection.Excluded) != 13 {
		t.Fatalf("selection = %#v", artifact.Selection)
	}
	if artifact.Selection.Selected[0].CaseID != "archive.org/fixture/case-00" || !artifact.Selection.Selected[0].RangePlayback {
		t.Fatalf("first selected = %#v", artifact.Selection.Selected[0])
	}
}

func TestBindReadyCandidateRejectsIncompleteRangeResponse(t *testing.T) {
	root := t.TempDir()
	row := catalogRow{hash: hashBytes([]byte("clip")), path: "clips/clip.mp4", disposition: "ready"}
	playbackBytes := []byte("playback")
	sourceBytes := []byte("source")
	writeFixture(t, filepath.Join(root, row.path), playbackBytes)
	writeFixture(t, filepath.Join(root, "masters/source.mp4"), sourceBytes)
	var sidecar clipSidecar
	sidecar.Loomarr.OriginalName = "clip.mp4"
	sidecar.Loomarr.MediaAssets.SourceMaster = mediaAsset{Path: "masters/source.mp4", SHA256: hashBytes(sourceBytes)}
	sidecar.Loomarr.MediaAssets.Playback.Asset = mediaAsset{Path: row.path, SHA256: hashBytes(playbackBytes)}
	sidecar.Loomarr.MediaAssets.Playback.QC.CompleteDecode = true
	sidecar.Loomarr.MediaAssets.Playback.QC.Seekable = true
	sidecar.Loomarr.MediaAssets.Playback.QC.FastStart = true
	sidecarBytes, err := json.Marshal(sidecar)
	if err != nil {
		t.Fatal(err)
	}
	writeFixture(t, filepath.Join(root, "clips/clip.info.json"), sidecarBytes)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte("partial without content range"))
	}))
	defer server.Close()

	_, err = bindReadyCandidate(context.Background(), assemblyConfig{
		FillerRoot: root,
		BaseURL:    server.URL,
		Client:     server.Client(),
	}, nil, row, fillerreleaseCandidate(row.hash))
	if err == nil || !strings.Contains(err.Error(), "range playback response is incomplete") {
		t.Fatalf("err = %v, want incomplete range response", err)
	}
}

func fillerreleaseCandidate(hash string) fillerrelease.CohortCandidate {
	return fillerrelease.CohortCandidate{
		CaseID: "archive.org/fixture/clip", ClipHash: hash, SourceIdentity: "archive.org/fixture", Disposition: "ready",
	}
}

func writeFixture(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}
