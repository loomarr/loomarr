//go:build ffmpeg

package testkit

import (
	"encoding/json"
	"os"
	"os/exec"
	"strconv"
	"testing"
)

func TestFillerConditioningMediaIsGeneratedLocallyAndBounded(t *testing.T) {
	fixtures := FillerConditioningMedia(t, t.TempDir())
	for name, path := range map[string]string{
		"compilation":     fixtures.Compilation,
		"offset loudness": fixtures.OffsetLoudness,
		"silent":          fixtures.Silent,
		"white frozen":    fixtures.WhiteFrozen,
		"short video":     fixtures.ShortVideo,
		"short audio":     fixtures.ShortAudio,
	} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("%s fixture: %v", name, err)
		}
		if !info.Mode().IsRegular() || info.Size() == 0 {
			t.Fatalf("%s fixture is not non-empty regular media: %+v", name, info)
		}
		assertFillerConditioningFixtureBounds(t, path)
	}
}

func assertFillerConditioningFixtureBounds(t *testing.T, path string) {
	t.Helper()
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Fatal("ffmpeg build-tag test requires ffprobe")
	}
	raw, err := exec.Command(ffprobe,
		"-v", "error", "-show_entries", "format=duration:stream=codec_type,width,height,avg_frame_rate",
		"-of", "json", path).Output()
	if err != nil {
		t.Fatal(err)
	}
	var probed struct {
		Streams []struct {
			Kind         string `json:"codec_type"`
			Width        int    `json:"width"`
			Height       int    `json:"height"`
			AvgFrameRate string `json:"avg_frame_rate"`
		} `json:"streams"`
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	if err := json.Unmarshal(raw, &probed); err != nil {
		t.Fatal(err)
	}
	duration, err := strconv.ParseFloat(probed.Format.Duration, 64)
	if err != nil || duration <= 0 || duration > 45 {
		t.Fatalf("fixture duration = %q, want 0s < duration <= 45s", probed.Format.Duration)
	}
	foundVideo := false
	for _, stream := range probed.Streams {
		if stream.Kind != "video" {
			continue
		}
		foundVideo = true
		if stream.Width != 320 || stream.Height != 180 || stream.AvgFrameRate != "30000/1001" {
			t.Fatalf("video stream = %+v, want 320x180 at exact 30000/1001", stream)
		}
	}
	if !foundVideo {
		t.Fatal("fixture has no video stream")
	}
}
