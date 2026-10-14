package mediameasure

import (
	"context"
	"io"
	"os"
	"testing"
)

type countingReaderAt struct {
	r     io.ReaderAt
	bytes int64
}

func (c *countingReaderAt) ReadAt(p []byte, off int64) (int, error) {
	n, err := c.r.ReadAt(p, off)
	c.bytes += int64(n)
	return n, err
}

// The Matroska index is read from the Cues element, not by scanning the file: the parsed keyframes
// match the ffprobe packet scan's timestamps, and only a small part of the file is read.
func TestMatroskaCuesMatchThePacketScanAndReadLittle(t *testing.T) {
	path := bigFixture(t)
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	info, _ := f.Stat()
	counted := &countingReaderAt{r: f}
	got, ok, err := matroskaKeyframes(counted, info.Size())
	if err != nil || !ok {
		t.Fatalf("matroskaKeyframes ok=%v err=%v", ok, err)
	}
	scan, err := DefaultTools("", "").scanKeyframes(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(scan) {
		t.Fatalf("cues gave %d keyframes, packet scan %d", len(got), len(scan))
	}
	for i := range got {
		if d := got[i].PTSMs - scan[i].PTSMs; d < -40 || d > 40 {
			t.Errorf("keyframe %d: cues %d ms, scan %d ms", i, got[i].PTSMs, scan[i].PTSMs)
		}
		if got[i].Offset <= 0 || (i > 0 && got[i].Offset < got[i-1].Offset) {
			t.Errorf("keyframe %d offset %d is not a forward cluster position", i, got[i].Offset)
		}
	}
	if counted.bytes > info.Size()/4 || counted.bytes > 3<<19 {
		t.Errorf("read %d of %d bytes: the index must be read, not the file scanned", counted.bytes, info.Size())
	}
	t.Logf("read %d of %d bytes for %d keyframes", counted.bytes, info.Size(), len(got))
}

func TestMatroskaKeyframesRejectsOtherContainers(t *testing.T) {
	path := t.TempDir() + "/x.mp4"
	if err := os.WriteFile(path, []byte("\x00\x00\x00\x18ftypmp42\x00\x00\x00\x00mp42isom"), 0o600); err != nil {
		t.Fatal(err)
	}
	f, _ := os.Open(path)
	defer func() { _ = f.Close() }()
	if _, ok, err := matroskaKeyframes(f, 24); ok || err != nil {
		t.Fatalf("mp4 reported as matroska: ok=%v err=%v", ok, err)
	}
}
