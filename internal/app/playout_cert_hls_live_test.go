//go:build ffmpeg

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/playoutcert"
)

func TestSyntheticTargetServesOrdinaryHLSAndJoinsRemux(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	target, err := NewPlayoutCertificationTarget(ctx, PlayoutCertificationConfig{
		Channels: []playoutcert.Channel{
			{ID: "cold-hls", Roles: []string{"transcode_h264", "audio_aac"}},
			{ID: "prepared-control", Roles: []string{"prepared"}},
		},
		FFmpeg: ffmpeg, Capacity: 1, Grace: 10 * time.Second, ProgrammeDuration: 6 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		closeCtx, closeCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer closeCancel()
		if err := target.Close(closeCtx); err != nil {
			t.Errorf("close target: %v", err)
		}
	}()
	request := func(method, address string, admin bool) (int, []byte) {
		t.Helper()
		req, err := http.NewRequestWithContext(ctx, method, address, strings.NewReader("{}"))
		if err != nil {
			t.Fatal("construct target request")
		}
		if admin {
			req.Header.Set("Authorization", "Bearer "+target.AdminBearer)
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal("target request failed") // Never print a signed URL.
		}
		defer func() { _ = resp.Body.Close() }()
		body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20+1))
		if err != nil || len(body) > 4<<20 {
			t.Fatal("target response unreadable or oversized")
		}
		return resp.StatusCode, body
	}
	frozenAt := time.Now()
	evidence, err := target.ProgrammeEvidence().Freeze("cold-hls", frozenAt, frozenAt.Add(45*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	status, body := request(http.MethodPost, target.BaseURL+"/v1/channels/cold-hls/play-url", true)
	var signed struct {
		RelativeURL string `json:"relativeUrl"`
	}
	if status != http.StatusOK || json.Unmarshal(body, &signed) != nil || signed.RelativeURL == "" {
		t.Fatalf("mint response status=%d", status)
	}
	base, err := url.Parse(target.BaseURL)
	if err != nil {
		t.Fatal("invalid target origin")
	}
	playlist, err := base.Parse(signed.RelativeURL)
	if err != nil || playlist.Host != base.Host || playlist.RawQuery == "" {
		t.Fatal("invalid signed playlist reference")
	}
	preparedOnly := *playlist
	query := preparedOnly.Query()
	query.Set("mode", "prepared")
	preparedOnly.RawQuery = query.Encode()
	if status, _ := request(http.MethodGet, preparedOnly.String(), false); status != http.StatusNoContent {
		t.Fatalf("cold prepared response=%d, want 204", status)
	}
	status, manifest := request(http.MethodGet, playlist.String(), false)
	if status != http.StatusOK || !bytes.HasPrefix(manifest, []byte("#EXTM3U")) {
		t.Fatalf("ordinary HLS response=%d, want media playlist", status)
	}
	var segment *url.URL
	for _, line := range strings.Split(string(manifest), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		reference, err := url.Parse(line)
		if err != nil || reference.IsAbs() || reference.Host != "" || strings.Contains(reference.Path, "..") || !strings.HasSuffix(reference.Path, ".ts") {
			t.Fatal("ordinary HLS did not reference a relative TS segment")
		}
		segment = playlist.ResolveReference(reference)
		break
	}
	if segment == nil {
		t.Fatal("ordinary HLS has no segment")
	}
	proof, err := evidence.ResolveAsset(ctx, playoutcert.ProgrammeAsset{Reference: segment.Path})
	if err != nil || len(proof.Media) == 0 || proof.Validate == nil || proof.Validate() != nil {
		t.Fatalf("private live reference unavailable: %v", err)
	}
	status, media := request(http.MethodGet, segment.String(), false)
	if status != http.StatusOK {
		t.Fatalf("HLS segment response=%d", status)
	}
	if !bytes.Equal(media, proof.Media) {
		t.Fatal("private reference differs from the signed response")
	}
	shape, err := (playoutcert.FFprobeValidator{}).Validate(ctx, media)
	if err != nil || shape.VideoStreams != 1 || shape.AudioStreams != 1 {
		t.Fatalf("ordinary HLS media shape=%+v error=%v", shape, err)
	}
	before, err := playoutcert.SampleResources(ctx, playoutcert.Config{
		BaseURL: target.BaseURL, AdminBearer: target.AdminBearer, DeviceToken: target.DeviceToken,
		Client: http.DefaultClient, RequestTimeout: 5 * time.Second,
	}, "before_shutdown")
	if err != nil || before.SessionsActive != 1 || before.FFmpegRunning < 2 {
		t.Fatalf("live remux/session resources=%+v error=%v", before, err)
	}
	target.origin.StopChannel("cold-hls")
	if proof.Validate() == nil {
		t.Fatal("stopped source still validates its reference")
	}
	if _, err := evidence.ResolveAsset(ctx, playoutcert.ProgrammeAsset{Reference: segment.Path}); err == nil {
		t.Fatal("retired asset resolved without a live remux")
	}
	if status, _ := request(http.MethodGet, playlist.String(), false); status != http.StatusOK {
		t.Fatalf("replacement HLS response=%d", status)
	}
	// Segment filenames are reused by the replacement remux. An existing
	// observation must not inherit its clock, even when the name is identical.
	if _, err := evidence.ResolveAsset(ctx, playoutcert.ProgrammeAsset{Reference: segment.Path}); err == nil {
		t.Fatal("existing observation accepted a replacement source")
	}
	restartedAt := time.Now()
	fresh, err := target.ProgrammeEvidence().Freeze("cold-hls", restartedAt, restartedAt.Add(20*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	replacement, err := fresh.ResolveAsset(ctx, playoutcert.ProgrammeAsset{Reference: segment.Path})
	if err != nil || replacement.Clock.Generation == proof.Clock.Generation || replacement.Validate == nil || replacement.Validate() != nil {
		t.Fatalf("fresh observation failed to bind replacement: %v", err)
	}
	status, media = request(http.MethodGet, segment.String(), false)
	if status != http.StatusOK || !bytes.Equal(media, replacement.Media) {
		t.Fatal("replacement signed response differs from its own reference")
	}
	receipt, err := target.Shutdown(ctx, playoutcert.ShutdownRequest{BaseURL: target.BaseURL})
	if err != nil || !receipt.ServingStopped || !receipt.ProcessesExited {
		t.Fatalf("shutdown receipt=%+v error=%v", receipt, err)
	}
	after, err := target.SampleStopped(ctx, "after_shutdown")
	if err != nil || after.SessionsActive != 0 || after.ViewerActive != 0 || after.FFmpegRunning != 0 {
		t.Fatalf("shutdown retained processes=%+v error=%v", after, err)
	}
	if err := target.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(target.root); !os.IsNotExist(err) {
		t.Fatalf("target scratch remains after disposal: %v", err)
	}
}
