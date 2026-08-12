package mediatools

import "testing"

const sampleFFmpegStderr = `Input #0, matroska,webm, from 'comp.mp4':
  Duration: 00:01:29.50, start: 0.000000, bitrate: 1200 kb/s
[blackdetect @ 0x7f9] black_start:29.8 black_end:30.2 black_duration:0.4
[blackdetect @ 0x7f9] black_start:59.9 black_end:60.1 black_duration:0.2
[silencedetect @ 0xabc] silence_start: 3.36
[silencedetect @ 0xabc] silence_end: 5.84 | silence_duration: 2.48
[silencedetect @ 0xabc] silence_start: 87.1
`

func TestParseBlackdetect(t *testing.T) {
	got := parseBlackdetect(sampleFFmpegStderr)
	if len(got) != 2 {
		t.Fatalf("black intervals = %+v, want 2", got)
	}
	if got[0] != (Interval{StartMs: 29800, EndMs: 30200}) {
		t.Errorf("first black interval = %+v", got[0])
	}
}

func TestParseSilencedetect_PairsStartsAndEnds(t *testing.T) {
	got := parseSilencedetect(sampleFFmpegStderr)
	if len(got) != 2 {
		t.Fatalf("silence intervals = %+v, want a pair + an unclosed tail", got)
	}
	if got[0] != (Interval{StartMs: 3360, EndMs: 5840}) {
		t.Errorf("paired silence = %+v", got[0])
	}
	// Trailing silence_start with no end (runs to EOF): kept, closed by the
	// duration clamp downstream — dropping it would miss the final boundary.
	if got[1].StartMs != 87100 || got[1].EndMs != got[1].StartMs {
		t.Errorf("unclosed tail = %+v", got[1])
	}
}

func TestParseFFprobeChapters_ScalesTimeBase(t *testing.T) {
	out := []byte(`{"chapters":[
		{"id":0,"time_base":"1/1000","start":0,"end":30000,"tags":{"title":"McDonald's"}},
		{"id":1,"time_base":"1/1000000","start":30000000,"end":61000000,"tags":{"title":"Lego"}}
	]}`)
	chs, err := parseFFprobeChapters(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(chs) != 2 {
		t.Fatalf("chapters = %+v", chs)
	}
	// The second chapter's time_base is 1/1000000 — assuming 1/1000 would put it
	// 1000× out. Scaling, not assuming, is the assertion.
	if chs[1].StartMs != 30000 || chs[1].EndMs != 61000 {
		t.Errorf("time_base not scaled: %+v", chs[1])
	}
	if chs[0].Title != "McDonald's" {
		t.Errorf("title lost: %+v", chs[0])
	}
}

func TestParseWhisperJSON(t *testing.T) {
	out := []byte(`{"transcription":[
		{"timestamps":{"from":"00:00:00,000","to":"00:00:02,000"},"offsets":{"from":0,"to":2000},"text":" Rice Krispies treats are so easy to make"},
		{"timestamps":{"from":"00:00:02,000","to":"00:00:04,500"},"offsets":{"from":2000,"to":4500},"text":" "},
		{"timestamps":{"from":"00:00:04,500","to":"00:00:06,000"},"offsets":{"from":4500,"to":6000},"text":" even the kids can do it"}
	]}`)
	segs, err := parseWhisperJSON(out)
	if err != nil {
		t.Fatal(err)
	}
	// The blank utterance is dropped; offsets (ms), not the timestamp strings,
	// are the source of truth.
	if len(segs) != 2 || segs[0].EndMs != 2000 || segs[1].StartMs != 4500 {
		t.Errorf("transcript = %+v", segs)
	}
	// NOTE: the prompt RENDERING of these segments (filler.TranscriptText) is asserted in
	// internal/filler — it formats for an LLM prompt, which is a domain concern, not a
	// property of parsing whisper output.
}

// --- rescue validation -------------------------------------------------------
