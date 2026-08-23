package filler_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/loomarr/loomarr/internal/filler"
)

func sidecarOf(t *testing.T, media string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(media[:len(media)-len(filepath.Ext(media))] + ".info.json")
	if err != nil {
		t.Fatalf("no sidecar beside %s: %v", media, err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

// ⚠ THE property that makes writing into someone else's file safe. `sidecarInfo` is a PARTIAL
// view — it declares only the fields we read — so a naive unmarshal/re-marshal would silently
// delete everything yt-dlp wrote that we do not name: formats, thumbnails, chapters, view counts.
// The operator would never see it happen.
func TestWriteSidecarTags_PreservesFieldsWeDoNotUnderstand(t *testing.T) {
	dir := t.TempDir()
	media := filepath.Join(dir, "coke.mp4")
	if err := os.WriteFile(media, []byte("video"), 0o600); err != nil {
		t.Fatal(err)
	}
	original := `{
	  "title": "Coke 1985",
	  "description": "A commercial",
	  "view_count": 4210,
	  "formats": [{"format_id": "22", "ext": "mp4"}],
	  "chapters": [{"start_time": 0, "title": "intro"}]
	}`
	if err := os.WriteFile(filepath.Join(dir, "coke.info.json"), []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := filler.WriteSidecarTags(media, filler.SidecarTags{Era: 1985, Audience: "kids"}, false); err != nil {
		t.Fatal(err)
	}

	doc := sidecarOf(t, media)
	for _, key := range []string{"title", "description", "view_count", "formats", "chapters"} {
		if _, ok := doc[key]; !ok {
			t.Errorf("writing tags DESTROYED %q — a partial struct round-trip deletes what it "+
				"does not declare", key)
		}
	}
	if doc["view_count"] != float64(4210) {
		t.Errorf("view_count = %v, want it untouched", doc["view_count"])
	}
}

// Tags round-trip, which is what makes migration 00033's tag loss recoverable.
func TestWriteSidecarTags_RoundTrips(t *testing.T) {
	dir := t.TempDir()
	media := filepath.Join(dir, "ad.mp4")
	if err := os.WriteFile(media, []byte("video"), 0o600); err != nil {
		t.Fatal(err)
	}

	want := filler.SidecarTags{SourceID: "archive:classic", Kind: "commercial", Era: 1992, Audience: "kids", Category: "toys", Confidence: 88}
	if err := filler.WriteSidecarTags(media, want, false); err != nil {
		t.Fatal(err)
	}
	got, ok := filler.ReadSidecarTags(media)
	if !ok {
		t.Fatal("tags did not read back")
	}
	if got != want {
		t.Errorf("round-trip lost data:\n got %+v\nwant %+v", got, want)
	}
}

func TestSidecarFetchedMarkFor_CarriesRegisteredSource(t *testing.T) {
	mark := filler.SidecarFetchedMarkFor("archive:classic")
	if mark["fetchedBy"] != "loomarr" || mark["sourceId"] != "archive:classic" {
		t.Fatalf("mark = %#v, want acquisition marker plus exact source id", mark)
	}
}

// Brand and transcript round-trip through the sidecar (§10 V44), so a catalog rebuild restores them
// rather than re-running the tagger (brand) or Whisper (transcript) over the whole folder — the same
// "metadata travels with the clip" rule originalName/normalizedLufs already follow.
func TestWriteSidecarTags_RoundTripsBrandAndTranscript(t *testing.T) {
	dir := t.TempDir()
	media := filepath.Join(dir, "coke.mp4")
	if err := os.WriteFile(media, []byte("video"), 0o600); err != nil {
		t.Fatal(err)
	}

	want := filler.SidecarTags{
		Kind:       "commercial",
		Era:        1985,
		Audience:   "general",
		Category:   "general",
		Brand:      "Coca-Cola",
		Transcript: "Have a Coke and a smile — the real thing.",
	}
	if err := filler.WriteSidecarTags(media, want, false); err != nil {
		t.Fatal(err)
	}
	got, ok := filler.ReadSidecarTags(media)
	if !ok {
		t.Fatal("tags did not read back")
	}
	if got != want {
		t.Errorf("brand/transcript round-trip lost data:\n got %+v\nwant %+v", got, want)
	}
}

// ⚠ An UNGROUNDED era must round-trip as a SUGGESTION, never as a tag. Restoring it as `Era`
// would launder a guess into a fact — the §8 failure the grounding rule exists to prevent, and
// the reason the two are separate fields rather than one.
func TestWriteSidecarTags_KeepsAnUngroundedEraASuggestion(t *testing.T) {
	dir := t.TempDir()
	media := filepath.Join(dir, "guess.mp4")
	if err := os.WriteFile(media, []byte("video"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := filler.WriteSidecarTags(media, filler.SidecarTags{SuggestedEra: 1985, Audience: "kids"}, false); err != nil {
		t.Fatal(err)
	}
	got, _ := filler.ReadSidecarTags(media)
	if got.Era != 0 {
		t.Errorf("Era = %d — a guess was laundered into a fact", got.Era)
	}
	if got.SuggestedEra != 1985 {
		t.Errorf("SuggestedEra = %d, want 1985", got.SuggestedEra)
	}
}

// ⚠ The held/filed fork's signal (V38c). It reads the FIELD, not the file's existence — which
// stopped working the moment Loomarr began writing sidecars for hand-dropped clips too.
func TestSidecarFetchedByUs_ReadsTheFieldNotTheFilesExistence(t *testing.T) {
	dir := t.TempDir()
	tagged := filepath.Join(dir, "dropped.mp4")
	fetched := filepath.Join(dir, "downloaded.mp4")
	for _, p := range []string{tagged, fetched} {
		if err := os.WriteFile(p, []byte("video"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	// A hand-dropped clip that Loomarr merely TAGGED: sidecar exists, but it was not fetched.
	if err := filler.WriteSidecarTags(tagged, filler.SidecarTags{Era: 1990}, false); err != nil {
		t.Fatal(err)
	}
	if filler.SidecarFetchedByUs(tagged) {
		t.Error("a hand-dropped clip reads as FETCHED — the fork is still keying on the file existing, " +
			"so every tagged clip would be held")
	}

	if err := filler.WriteSidecarTags(fetched, filler.SidecarTags{}, true); err != nil {
		t.Fatal(err)
	}
	if !filler.SidecarFetchedByUs(fetched) {
		t.Error("a downloaded clip does not read as fetched")
	}
}

// ⚠ A later tagging pass must not UN-mark a fetched clip. Doing so would flip its lifecycle on
// the next scan — exactly the fragility the field replaced.
func TestWriteSidecarTags_TaggingDoesNotClearFetchedBy(t *testing.T) {
	dir := t.TempDir()
	media := filepath.Join(dir, "clip.mp4")
	if err := os.WriteFile(media, []byte("video"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := filler.WriteSidecarTags(media, filler.SidecarTags{}, true); err != nil {
		t.Fatal(err)
	}
	// The tagger runs later and knows nothing about how the clip arrived.
	if err := filler.WriteSidecarTags(media, filler.SidecarTags{Era: 1985}, false); err != nil {
		t.Fatal(err)
	}
	if !filler.SidecarFetchedByUs(media) {
		t.Error("a tagging pass cleared fetchedBy — the clip would file itself on the next scan")
	}
}

// ⚠ A sidecar that exists but does not parse is LEFT ALONE. It is the operator's file, and
// replacing something we cannot read is the one move guaranteed to destroy information.
func TestWriteSidecarTags_RefusesToOverwriteUnreadableJSON(t *testing.T) {
	dir := t.TempDir()
	media := filepath.Join(dir, "clip.mp4")
	if err := os.WriteFile(media, []byte("video"), 0o600); err != nil {
		t.Fatal(err)
	}
	broken := filepath.Join(dir, "clip.info.json")
	if err := os.WriteFile(broken, []byte(`{not json at all`), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := filler.WriteSidecarTags(media, filler.SidecarTags{Era: 1985}, false); err == nil {
		t.Error("overwrote an unparseable sidecar instead of refusing")
	}
	raw, _ := os.ReadFile(broken)
	if string(raw) != `{not json at all` {
		t.Error("the operator's unreadable file was modified")
	}
}
