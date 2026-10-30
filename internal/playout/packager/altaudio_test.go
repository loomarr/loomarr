package packager

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
)

// The alternate audio (#2088 decision 6) rides the encoder's stream as track 3 and leaves the
// packager as a rendition of its own: each segment is written twice, the main file (video and the
// default audio, what tuners and the variant playlist read) and an alt- file holding only track 3,
// on the same timeline as the default audio. Trims, the priming drop and slate apply to both alike.
func TestAlternateAudioIsItsOwnRendition(t *testing.T) {
	never := make(chan struct{})
	h := runPlans(t, Config{SlateLead: 1500 * time.Millisecond, URIPrefix: "baseline-"}, []plan{
		{"prog", 2500 * time.Millisecond, synth{label: "prog", alt: true}},
		{"late", time.Second, synth{label: "late", alt: true, blockBefore: never}},
		{"ad", 1200 * time.Millisecond, synth{label: "ad", alt: true, extraAudio: 3}},
	})
	// The main rendition is the timeline it always was, with no track 3 in it.
	_, mainFrames := assertGapless(t, h)

	var init fmp4.Init
	if err := init.Unmarshal(bytes.NewReader(h.p.Init())); err != nil || len(init.Tracks) != 2 {
		t.Fatalf("served init: %v, %d tracks; want video and the default audio", err, len(init.Tracks))
	}
	alt, err := os.ReadFile(filepath.Join(h.dir, AltInitName))
	if err != nil {
		t.Fatal(err)
	}
	var altInit fmp4.Init
	if err := altInit.Unmarshal(bytes.NewReader(alt)); err != nil || len(altInit.Tracks) != 1 || altInit.Tracks[0].ID != altTrack {
		t.Fatalf("alt init: %v, %+v; want track 3 alone", err, altInit.Tracks)
	}

	h.p.mu.Lock()
	segs := append([]segment(nil), h.p.window.segs...)
	h.p.mu.Unlock()
	_, mainAudio, _ := h.segments()
	var next uint64
	frames := 0
	for i, s := range segs {
		b, err := os.ReadFile(filepath.Join(h.dir, s.alt))
		if err != nil {
			t.Fatalf("segment %d: %v", i, err)
		}
		var parts fmp4.Parts
		if err := parts.Unmarshal(b); err != nil || len(parts) != 1 || len(parts[0].Tracks) != 1 {
			t.Fatalf("%s: %v; want one part with one track", s.alt, err)
		}
		tr := parts[0].Tracks[0]
		if tr.ID != altTrack || tr.BaseTime != next || tr.BaseTime != mainAudio[i].base {
			t.Fatalf("%s: track %d at %d; want track 3 at %d, beside the default audio", s.alt, tr.ID, tr.BaseTime, next)
		}
		if len(tr.Samples) != len(mainAudio[i].samples) {
			t.Fatalf("%s: %d frames beside %d of the default audio", s.alt, len(tr.Samples), len(mainAudio[i].samples))
		}
		for j, smp := range tr.Samples {
			// The same frame as the default audio's, from the alternate track: a1 beside b1.
			want := strings.Replace(string(mainAudio[i].samples[j].Payload), "/a", "/b", 1)
			if string(smp.Payload) != want || smp.Duration != aacFrame {
				t.Fatalf("%s frame %d: %q (%d samples), want %q", s.alt, j, smp.Payload, smp.Duration, want)
			}
		}
		next += uint64(len(tr.Samples)) * aacFrame
		frames += len(tr.Samples)
	}
	if frames != mainFrames {
		t.Fatalf("the alternate carries %d frames, the default %d", frames, mainFrames)
	}

	// Its playlist lists the same segments under their alt- names, behind its own init.
	pl := string(h.p.AltPlaylist())
	if !strings.Contains(pl, `#EXT-X-MAP:URI="baseline-alt-init.mp4"`) || !strings.Contains(pl, "baseline-"+segs[0].alt) ||
		strings.Count(pl, "#EXTINF") != strings.Count(string(h.p.Playlist()), "#EXTINF") {
		t.Fatalf("alt playlist:\n%s", pl)
	}
	if !h.p.HasAlt() {
		t.Fatal("HasAlt is false on a channel whose init carries track 3")
	}
}

// Every segment is split, so the encoder's layout (one trun per traf, track 3's samples last) must
// take the byte path, never the re-marshal; and the two must agree sample for sample.
func TestSplitInPlaceTakesTheEncoderLayout(t *testing.T) {
	stream, err := io.ReadAll(synth{label: "x", frames: testFPS, alt: true}.encode(t, context.Background(), Slot{Frames: testFPS}))
	if err != nil {
		t.Fatal(err)
	}
	s := readStream(bytes.NewReader(stream))
	frag := <-s.frags
	if _, err := patchFragment(frag, 7, map[uint32]uint64{videoTrack: 900, audioTrack: 480, altTrack: 480}); err != nil {
		t.Fatal(err)
	}
	main, alt, ok := splitInPlace(frag)
	if !ok {
		t.Fatal("the encoder's layout fell back to re-marshalling")
	}
	wantMain, wantAlt, err := splitByMarshal(frag)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ got, want []byte }{{main, wantMain}, {alt, wantAlt}} {
		got, err := unmarshalFragment(c.got)
		if err != nil {
			t.Fatal(err)
		}
		want, err := unmarshalFragment(c.want)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("in place %+v\nre-marshalled %+v", got[0].Tracks, want[0].Tracks)
		}
	}
	// And a fragment without track 3 is passed through untouched.
	plain, err := io.ReadAll(synth{label: "y", frames: testFPS}.encode(t, context.Background(), Slot{Frames: testFPS}))
	if err != nil {
		t.Fatal(err)
	}
	f := <-readStream(bytes.NewReader(plain)).frags
	if m, a, err := splitFragment(f); err != nil || a != nil || &m[0] != &f[0] {
		t.Fatalf("a two-track fragment was split: alt %d bytes, err %v", len(a), err)
	}
}

// testdata/ffmpeg-fragmented-alt.mp4 is real ffmpeg output in the encoder contract's shape (-movflags
// empty_moov+default_base_moof+frag_keyframe, 33 frames of libx264, two AAC tracks): what every
// production segment is. Each of its fragments must take the byte path, and its init must split.
func TestSplitInPlaceTakesFFmpegsOwnLayout(t *testing.T) {
	raw, err := os.ReadFile("testdata/ffmpeg-fragmented-alt.mp4")
	if err != nil {
		t.Fatal(err)
	}
	stream := readStream(bytes.NewReader(raw))
	n := 0
	for frag := range stream.frags {
		n++
		main, alt, ok := splitInPlace(frag)
		if !ok || alt == nil {
			t.Fatalf("fragment %d: ffmpeg's layout fell back to re-marshalling", n)
		}
		wantMain, wantAlt, err := splitByMarshal(frag)
		if err != nil {
			t.Fatal(err)
		}
		// Compared as the packager reads fragments: ffmpeg marks the IDR in trun's first_sample_flags,
		// which the byte path keeps as it was and mediacommon's own parse would not see.
		for _, c := range []struct{ got, want []byte }{{main, wantMain}, {alt, wantAlt}} {
			got, err := unmarshalFragment(c.got)
			if err != nil {
				t.Fatalf("fragment %d: %v", n, err)
			}
			want, err := unmarshalFragment(c.want)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("fragment %d: in place and re-marshalled differ", n)
			}
			if v := got[0].Tracks[0]; v.ID == videoTrack && v.Samples[0].IsNonSyncSample {
				t.Fatalf("fragment %d: the split lost the IDR", n)
			}
		}
	}
	if stream.err != nil || n < 2 {
		t.Fatalf("%d fragments, err %v", n, stream.err)
	}
	main, alt, ok := splitInit(stream.init)
	var mi, ai fmp4.Init
	if !ok || mi.Unmarshal(bytes.NewReader(main)) != nil || ai.Unmarshal(bytes.NewReader(alt)) != nil ||
		len(mi.Tracks) != 2 || len(ai.Tracks) != 1 || ai.Tracks[0].ID != altTrack {
		t.Fatalf("init split: ok %v, main %+v, alt %+v", ok, mi.Tracks, ai.Tracks)
	}
}

// A channel without the alternate (every encoder before #2088 step 9, and every test fixture) is
// untouched: one file per segment, no alt init, and HasAlt false.
func TestNoAlternateWritesOneFilePerSegment(t *testing.T) {
	h := runPlans(t, Config{}, []plan{{"prog", 2 * time.Second, synth{label: "prog"}}})
	assertGapless(t, h)
	if h.p.HasAlt() {
		t.Fatal("HasAlt on a two-track channel")
	}
	if _, err := os.Stat(filepath.Join(h.dir, AltInitName)); !os.IsNotExist(err) {
		t.Fatalf("alt init written: %v", err)
	}
	h.p.mu.Lock()
	defer h.p.mu.Unlock()
	for _, s := range h.p.window.segs {
		if s.alt != "" {
			t.Fatalf("segment %s has an alternate %s", s.name, s.alt)
		}
	}
}
