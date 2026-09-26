package mediameasure

import (
	"context"
	"os"
	"testing"
)

// The MP4 index is read from the sample tables (stss/stts/stsc/stsz/stco): timestamps and exact
// byte offsets match the ffprobe packet scan, and the mdat is never read.
func TestMP4SampleTablesMatchThePacketScanAndSkipMdat(t *testing.T) {
	path := mp4Fixture(t)
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	info, _ := f.Stat()
	counted := &countingReaderAt{r: f}
	got, ok, err := mp4Keyframes(counted, info.Size())
	if err != nil || !ok {
		t.Fatalf("mp4Keyframes ok=%v err=%v", ok, err)
	}
	scan, err := DefaultTools("", "").scanKeyframes(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(scan) || len(got) != 30 {
		t.Fatalf("sample tables gave %d keyframes, packet scan %d, want 30", len(got), len(scan))
	}
	for i := range got {
		if d := got[i].PTSMs - scan[i].PTSMs; d < -40 || d > 40 {
			t.Errorf("keyframe %d: tables %d ms, scan %d ms", i, got[i].PTSMs, scan[i].PTSMs)
		}
		if got[i].Offset != scan[i].Offset {
			t.Errorf("keyframe %d: offset %d, scan %d", i, got[i].Offset, scan[i].Offset)
		}
	}
	if counted.bytes > info.Size()/4 {
		t.Errorf("read %d of %d bytes: the mdat must be skipped", counted.bytes, info.Size())
	}
	t.Logf("read %d of %d bytes for %d keyframes", counted.bytes, info.Size(), len(got))
}

func TestMP4KeyframesRejectsMatroska(t *testing.T) {
	f, err := os.Open(fadeFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	info, _ := f.Stat()
	if _, ok, err := mp4Keyframes(f, info.Size()); ok || err != nil {
		t.Fatalf("matroska reported as mp4: ok=%v err=%v", ok, err)
	}
}
