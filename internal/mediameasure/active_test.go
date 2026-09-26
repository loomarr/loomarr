package mediameasure

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// A 2.39:1 film with its letterbox baked into a 16:9 frame: one scene is dark (cropdetect crops
// into the picture), the others lit. The union is the picture; the dark scene never shrinks it.
func TestActivePicture_UnionAcrossSampledScenes(t *testing.T) {
	boxes := []string{"crop=1920:800:0:140", "crop=1600:560:160:260", "crop=1920:804:0:138", "", "crop=1920:800:0:140"}
	calls := 0
	var seeks []string
	tools := Tools{FFmpeg: "ffmpeg", Run: func(_ context.Context, _ string, args ...string) ([]byte, []byte, error) {
		for i, a := range args {
			if a == "-ss" {
				seeks = append(seeks, args[i+1])
			}
			if a == "-frames:v" && args[i+1] != "12" {
				t.Errorf("decodes %s frames per sample, want 12", args[i+1])
			}
		}
		b := boxes[calls]
		calls++
		if b == "" {
			return nil, []byte("error"), fmt.Errorf("exit 1")
		}
		// cropdetect's running answer: the last line is the one that counts.
		return nil, []byte("[Parsed_cropdetect_0] x1:0 crop=1920:1080:0:0\n[Parsed_cropdetect_0] " + b + "\n"), nil
	}}
	got, ok := tools.ActivePicture(context.Background(), "/film.mkv", 7_200_000, 1920, 1080, DefaultActiveSampling())
	if !ok || got != (Box{X: 0, Y: 138, W: 1920, H: 804}) {
		t.Errorf("active picture %+v ok=%v", got, ok)
	}
	if calls != 5 || strings.Join(seeks, ",") != "1200.000,2400.000,3600.000,4800.000,6000.000" {
		t.Errorf("sampled %d points at %v; want 5 spread through the programme", calls, seeks)
	}
}

func TestLastCrop_RejectsNoAnswer(t *testing.T) {
	for _, s := range []string{"", "crop=-1920:-1072:1926:1076", "crop=1920:1088:0:0"} {
		if b, ok := lastCrop(s, 1920, 1080); ok {
			t.Errorf("%q gave %+v", s, b)
		}
	}
}
