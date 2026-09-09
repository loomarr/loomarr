//go:build ffmpeg

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/playoutcert"
	"github.com/loomarr/loomarr/internal/store"
)

// Exercise the persisted channel policy through real routes and media. The
// generated sources are H264, so HEVC output must come from an actual encoder.
func TestCertificationTargetHonorsStoredBroadcastCodec(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg unavailable")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	target, err := NewPlayoutCertificationTarget(ctx, PlayoutCertificationConfig{
		Channels: []playoutcert.Channel{
			{ID: "codec-channel", Roles: []string{"transcode_h264", "audio_aac"}},
			{ID: "prepared-control", Roles: []string{"prepared"}},
		},
		FFmpeg: ffmpeg, Capacity: 1, Grace: 100 * time.Millisecond, ProgrammeDuration: 30 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		closeCtx, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		if err := target.Close(closeCtx); err != nil {
			t.Error(err)
		}
	}()
	channel, err := target.store.GetChannel(ctx, "codec-channel")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := target.store.SetChannelBroadcastCodec(ctx, channel.ID, channel.Revision, store.BroadcastCodecHEVC); err != nil {
		t.Fatal(err)
	}
	request := func(t *testing.T, method, address, body string, limit int64, finite bool) []byte {
		t.Helper()
		req, err := http.NewRequestWithContext(ctx, method, address, strings.NewReader(body))
		if err != nil {
			t.Fatal("construct certification request")
		}
		if method == http.MethodPost {
			req.Header.Set("Authorization", "Bearer "+target.AdminBearer)
			req.Header.Set("Content-Type", "application/json")
		}
		response, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal("certification request failed") // Never print signed URLs.
		}
		defer func() { _ = response.Body.Close() }()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("certification response status=%d", response.StatusCode)
		}
		data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
		if err != nil || (finite && int64(len(data)) > limit) {
			t.Fatal("unreadable or oversized certification response")
		}
		return data
	}
	for _, tc := range []struct {
		name, capabilities, plan, codec string
	}{
		{name: "native tuner", codec: "hevc"},
		{name: "native HLS", capabilities: `{"video":["hevc"]}`, plan: "hevc8", codec: "hevc"},
		{name: "baseline HLS", capabilities: `{}`, codec: "h264"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defer target.origin.StopChannel(channel.ID)
			var media []byte
			if tc.capabilities == "" {
				media = request(t, http.MethodGet, target.BaseURL+"/v1/playout/stream/"+channel.ID+"?token="+url.QueryEscape(target.DeviceToken), "", 256<<10, false)
			} else {
				body := request(t, http.MethodPost, target.BaseURL+"/v1/channels/"+channel.ID+"/play-url", tc.capabilities, 1<<20, true)
				var signed struct {
					RelativeURL string `json:"relativeUrl"`
				}
				if json.Unmarshal(body, &signed) != nil || signed.RelativeURL == "" {
					t.Fatal("invalid play URL response")
				}
				playlist, err := url.Parse(target.BaseURL + signed.RelativeURL)
				if err != nil || playlist.Query().Get("plan") != tc.plan {
					t.Fatal("minted URL did not negotiate the expected channel/client plan")
				}
				manifest := request(t, http.MethodGet, playlist.String(), "", 1<<20, true)
				var initRef, segmentRef string
				for _, line := range strings.Split(string(manifest), "\n") {
					line = strings.TrimSpace(line)
					if strings.HasPrefix(line, `#EXT-X-MAP:URI="`) {
						initRef, _, _ = strings.Cut(strings.TrimPrefix(line, `#EXT-X-MAP:URI="`), `"`)
					}
					if line != "" && !strings.HasPrefix(line, "#") && segmentRef == "" {
						segmentRef = line
					}
				}
				if segmentRef == "" || (initRef != "") != (tc.codec == "hevc") {
					t.Fatal("negotiated HLS packaging does not match the codec")
				}
				for _, reference := range []string{initRef, segmentRef} {
					if reference == "" {
						continue
					}
					asset, err := playlist.Parse(reference)
					if err != nil || asset.Scheme != playlist.Scheme || asset.Host != playlist.Host {
						t.Fatal("invalid HLS asset origin")
					}
					media = append(media, request(t, http.MethodGet, asset.String(), "", 4<<20, true)...)
				}
			}
			shape, err := (playoutcert.FFprobeValidator{}).Validate(ctx, media)
			if err != nil || shape.VideoStreams != 1 || shape.AudioStreams != 1 || shape.VideoCodec != tc.codec || shape.AudioCodec != "aac" {
				t.Fatalf("expected %s/AAC output: shape=%+v error=%v", tc.codec, shape, err)
			}
			var frames, audioSamples int64
			decodeErr := (playoutcert.FFmpegSignalDecoder{Path: ffmpeg}).DecodeSignals(ctx, io.NopCloser(bytes.NewReader(media)),
				func(playoutcert.DecodedVideoSignal) { frames++ },
				func(audio playoutcert.DecodedAudioSignal) { audioSamples += audio.Samples })
			if decodeErr != nil || frames < 10 || audioSamples < 12000 {
				t.Fatalf("output did not decode: video=%d audio=%d error=%v", frames, audioSamples, decodeErr)
			}
			t.Logf("output=%s/aac decoded video=%d audio samples=%d", tc.codec, frames, audioSamples)
		})
		if t.Failed() {
			break
		}
	}
}
