package packager

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
)

// testHDR10 is a stand-in for playout.ChannelHDR10: the packager treats the payloads as opaque.
var testHDR10 = &HDR10{
	SEI:  []byte{0, 0, 0, 1, 39 << 1, 1, 137, 24, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 0x80},
	MDCV: bytes.Repeat([]byte{0xab}, 24),
	CLLI: []byte{0x03, 0xe8, 0x01, 0x90},
	Colr: []byte{'n', 'c', 'l', 'x', 0, 9, 0, 16, 0, 9, 0},
}

// hevcFixture is real ffmpeg output: libx265 Main 10 tagged BT.2020 / PQ, an IDR every 2 frames
// (4 frames, 2 fragments), plus AAC, with the packager's movflags. x265 puts its own user-data
// prefix SEI in the first IDR, the neighbour the static SEI must be placed after.
func hevcFixture(t *testing.T) (init []byte, frags [][]byte) {
	t.Helper()
	raw, err := os.ReadFile("testdata/hevc-main10-fragmented.mp4")
	if err != nil {
		t.Fatal(err)
	}
	s := readStream(bytes.NewReader(raw))
	for f := range s.frags {
		frags = append(frags, f)
	}
	if s.err != nil || len(frags) != 2 {
		t.Fatalf("fixture: %d fragments, err %v", len(frags), s.err)
	}
	return s.init, frags
}

// sampleEntryBoxes returns the child boxes of the init's HEVC sample entry, by type.
func sampleEntryBoxes(t *testing.T, init []byte) map[string][][]byte {
	t.Helper()
	got := map[string][][]byte{}
	for _, stsd := range SampleDescriptions(init) {
		children(stsd, 16, len(stsd), func(typ string, s, sz int) {
			if typ != "hvc1" && typ != "hev1" {
				return
			}
			children(stsd, s+visualSampleEntryHeader, s+sz, func(ct string, cs, csz int) {
				got[ct] = append(got[ct], stsd[cs+8:cs+csz])
			})
		})
	}
	return got
}

// The init's HEVC sample entry carries the channel's mdcv and clli, and exactly one colr: the
// encoder's own when it wrote one. The result still parses as the same HEVC track.
func TestHDR10InitCarriesTheStaticBoxes(t *testing.T) {
	init, _ := hevcFixture(t)
	out, err := testHDR10.init(init)
	if err != nil {
		t.Fatal(err)
	}
	boxes := sampleEntryBoxes(t, out)
	if len(boxes["mdcv"]) != 1 || !bytes.Equal(boxes["mdcv"][0], testHDR10.MDCV) {
		t.Errorf("mdcv = %x, want one carrying %x", boxes["mdcv"], testHDR10.MDCV)
	}
	if len(boxes["clli"]) != 1 || !bytes.Equal(boxes["clli"][0], testHDR10.CLLI) {
		t.Errorf("clli = %x, want one carrying %x", boxes["clli"], testHDR10.CLLI)
	}
	if len(boxes["colr"]) != 1 || len(boxes["hvcC"]) != 1 {
		t.Errorf("sample entry: %d colr, %d hvcC; want one of each", len(boxes["colr"]), len(boxes["hvcC"]))
	}
	var before, after fmp4.Init
	if err := before.Unmarshal(bytes.NewReader(init)); err != nil {
		t.Fatal(err)
	}
	if err := after.Unmarshal(bytes.NewReader(out)); err != nil {
		t.Fatalf("boxed init does not parse: %v", err)
	}
	if len(after.Tracks) != len(before.Tracks) {
		t.Fatalf("tracks %d, want %d", len(after.Tracks), len(before.Tracks))
	}
	// Idempotent: a boxed init is left as it is.
	if again, err := testHDR10.init(out); err != nil || !bytes.Equal(again, out) {
		t.Errorf("boxing a boxed init changed it (err %v)", err)
	}
}

// nals splits a length-prefixed (4-byte) HEVC sample into its NAL units.
func nals(t *testing.T, sample []byte) [][]byte {
	t.Helper()
	var out [][]byte
	for off := 0; off < len(sample); {
		if off+4 > len(sample) {
			t.Fatalf("truncated NAL length at %d", off)
		}
		n := int(binary.BigEndian.Uint32(sample[off:]))
		if off+4+n > len(sample) {
			t.Fatalf("NAL overruns the sample at %d", off)
		}
		out = append(out, sample[off+4:off+4+n])
		off += 4 + n
	}
	return out
}

// Every IDR access unit carries the static SEI once, before its first slice and after whatever
// non-VCL units the encoder put first; every other sample, and the audio, is untouched.
func TestHDR10SEIPrefixesEveryIDR(t *testing.T) {
	_, frags := hevcFixture(t)
	want := testHDR10.SEI[4:] // the NAL, start code dropped
	idrs := 0
	for i, frag := range frags {
		out, err := testHDR10.fragment(append([]byte(nil), frag...))
		if err != nil {
			t.Fatal(err)
		}
		before, err := unmarshalFragment(frag)
		if err != nil {
			t.Fatal(err)
		}
		after, err := unmarshalFragment(out)
		if err != nil {
			t.Fatalf("fragment %d: the result does not parse: %v", i, err)
		}
		for p := range before {
			for tr := range before[p].Tracks {
				bs, as := before[p].Tracks[tr].Samples, after[p].Tracks[tr].Samples
				if len(bs) != len(as) {
					t.Fatalf("fragment %d track %d: %d samples, want %d", i, before[p].Tracks[tr].ID, len(as), len(bs))
				}
				for s := range bs {
					if as[s].IsNonSyncSample != bs[s].IsNonSyncSample {
						t.Errorf("fragment %d sample %d: sync flag changed", i, s)
					}
					if before[p].Tracks[tr].ID != videoTrack || bs[s].IsNonSyncSample {
						if !bytes.Equal(as[s].Payload, bs[s].Payload) {
							t.Errorf("fragment %d track %d sample %d: changed, want untouched", i, before[p].Tracks[tr].ID, s)
						}
						continue
					}
					idrs++
					units := nals(t, as[s].Payload)
					seiAt, firstVCL, count := -1, -1, 0
					for k, u := range units {
						if bytes.Equal(u, want) {
							seiAt, count = k, count+1
						}
						if firstVCL < 0 && (u[0]>>1)&0x3f <= 31 {
							firstVCL = k
						}
					}
					if count != 1 || seiAt < 0 || seiAt != firstVCL-1 {
						t.Errorf("fragment %d IDR: static SEI %d times at %d, first slice at %d; want once, just before it",
							i, count, seiAt, firstVCL)
						continue
					}
					rest := append([][]byte(nil), units[:seiAt]...)
					rest = append(rest, units[seiAt+1:]...)
					if !bytes.Equal(bytes.Join(rest, nil), bytes.Join(nals(t, bs[s].Payload), nil)) {
						t.Errorf("fragment %d IDR: other NAL units changed", i)
					}
				}
			}
		}
	}
	if idrs != 2 {
		t.Fatalf("checked %d IDRs, want the fixture's 2", idrs)
	}
}

// The SEI goes after an access unit's leading non-VCL units (an AUD, the encoder's own SEI) and
// before its first slice; a unit already carrying it, or a broken one, is not given a second.
func TestWithPrefixSEIPlacement(t *testing.T) {
	unit := func(typ byte, body ...byte) []byte {
		return append(binary.BigEndian.AppendUint32(nil, uint32(2+len(body))), append([]byte{typ << 1, 1}, body...)...)
	}
	aud, own, slice := unit(35, 0x50), unit(39, 5, 1, 0x80), unit(19, 0xaf, 0x01)
	static := unit(39, 137, 1, 0x80)
	au := bytes.Join([][]byte{aud, own, slice}, nil)
	got, err := withPrefixSEI(au, static)
	if want := bytes.Join([][]byte{aud, own, static, slice}, nil); err != nil || !bytes.Equal(got, want) {
		t.Fatalf("got %x (err %v), want %x", got, err, want)
	}
	if again, err := withPrefixSEI(got, static); err != nil || !bytes.Equal(again, got) {
		t.Errorf("a second insert changed the access unit (err %v)", err)
	}
	if _, err := withPrefixSEI(bytes.Join([][]byte{aud, own}, nil), static); err == nil {
		t.Error("an access unit with no slice was accepted")
	}
	if _, err := withPrefixSEI(append(binary.BigEndian.AppendUint32(nil, 99), 0x26, 1), static); err == nil {
		t.Error("a NAL length overrunning the sample was accepted")
	}
}

// A packager configured for HDR10 serves the boxed init and writes every segment with the SEI;
// its decoder comparison still reads the encoder's own init, so the next item's raw init joins.
// The HDR10 boxes are the packager's, not the encoder's: a slate from the channel's own pipeline
// has the encoder's raw init, so it is compared against that, never against the served init. Live
// on NVENC every slate of a 4K HDR channel counted as a decoder mismatch before this.
func TestHDR10SlateMatchesTheEncodersInit(t *testing.T) {
	init, _ := hevcFixture(t)
	slate := &Slate{init: append([]byte(nil), init...)} // the check precedes any sample
	idle := func(context.Context, time.Time) (Item, error) { return Item{}, nil }
	p, err := New(Config{FPS: 25, Dir: t.TempDir(), HDR10: testHDR10}, idle, readySlate(slate))
	if err != nil {
		t.Fatal(err)
	}
	if !p.acceptInit(init) {
		t.Fatal("the encoder's init was refused")
	}
	p.epoch = time.Now()
	if err := p.fillSlate(t.Context(), Slot{}); err != nil {
		t.Fatal(err)
	}
	if s := p.Stats(); s.Slates != 1 || s.DecoderMismatch != 0 {
		t.Fatalf("stats %+v: want one slate that matches the channel", s)
	}
}

func TestPackagerWritesHDR10IntoInitAndSegments(t *testing.T) {
	init, frags := hevcFixture(t)
	dir := t.TempDir()
	idle := func(context.Context, time.Time) (Item, error) { return Item{}, nil }
	p, err := New(Config{FPS: 25, Dir: dir, HDR10: testHDR10}, idle, readySlate(nil))
	if err != nil {
		t.Fatal(err)
	}
	if !p.acceptInit(init) || !p.acceptInit(append([]byte(nil), init...)) {
		t.Fatal("the encoder's init was refused")
	}
	onDisk, err := os.ReadFile(filepath.Join(dir, InitName))
	if err != nil {
		t.Fatal(err)
	}
	if len(sampleEntryBoxes(t, p.Init())["mdcv"]) != 1 || !bytes.Equal(onDisk, p.Init()) {
		t.Errorf("served init: mdcv %d; file equals Init() %v", len(sampleEntryBoxes(t, p.Init())["mdcv"]), bytes.Equal(onDisk, p.Init()))
	}
	p.epoch = time.Now() // Run's anchor: the segment airs now, well inside the run-ahead
	c, err := patchFragment(frags[0], 0, map[uint32]uint64{videoTrack: 0, audioTrack: 0})
	if err != nil {
		t.Fatal(err)
	}
	if err := p.appendSegment(t.Context(), frags[0], c); err != nil {
		t.Fatal(err)
	}
	seg, err := os.ReadFile(filepath.Join(dir, segmentName(0)))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(seg, testHDR10.SEI[4:]) {
		t.Error("the written segment has no static SEI")
	}
}
