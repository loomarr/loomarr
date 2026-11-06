package rating

import "testing"

// The ladder orders TV and film systems together; unmappable → unrated.
func TestLadder(t *testing.T) {
	if !Rating("TV-Y7").AtOrBelow("TV-14") {
		t.Error("TV-Y7 should be at or below TV-14")
	}
	if Rating("TV-MA").AtOrBelow("TV-Y7") {
		t.Error("TV-MA must NOT be at or below TV-Y7")
	}
	if !Rating("PG").AtOrBelow("PG-13") {
		t.Error("PG (film) should be at or below PG-13 (film) on the shared ladder")
	}
	// Cross-system: G (film) and TV-G share a rank.
	if !Rating("G").AtOrBelow("TV-G") || !Rating("TV-G").AtOrBelow("G") {
		t.Error("G and TV-G should share a ladder rank")
	}
	// An unmapped rating is never "at or below" anything — it's unrated, decided by policy.
	if Rating("XYZ").AtOrBelow("TV-MA") {
		t.Error("an unmapped rating must not compare as at-or-below")
	}
}

// Normalize folds the common media-server string variants onto the ladder.
func TestNormalize(t *testing.T) {
	cases := map[string]Rating{
		"TV-Y7":   "TV-Y7",
		"tv-y7":   "TV-Y7",
		"TV_Y7":   "TV-Y7",
		"Rated R": "R",
		" pg-13 ": "PG-13",
		"NR":      "", // not rated → unrated
		"Unknown": "",
	}
	for raw, want := range cases {
		if got := Normalize(raw); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", raw, got, want)
		}
	}
}

// FuzzNormalize asserts two invariants of Normalize over arbitrary raw strings:
//   - idempotence: Normalize(Normalize(x)) == Normalize(x). Once folded onto the ladder (or to
//     unrated ""), re-folding is a no-op.
//   - totality: the result is always either "" (unrated) or an on-ladder rating — never some
//     half-normalized off-ladder string.
func FuzzNormalize(f *testing.F) {
	for _, s := range []string{
		"TV-Y7", "tv-y7", "TV_Y7", "Rated R", " pg-13 ", "NR", "Unknown",
		"", "   ", "rated tv-ma", "PG", "g", "TV-MA", "NC-17", "RATEDR",
		"____", "TV Y7", "nc_17", "\x00", "🎬", "tv-14 ",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		once := Normalize(raw)
		if once != "" && !once.Mapped() {
			t.Fatalf("Normalize(%q) = %q, which is neither \"\" nor on the ladder", raw, once)
		}
		if twice := Normalize(string(once)); twice != once {
			t.Fatalf("Normalize not idempotent: Normalize(%q)=%q, Normalize(%q)=%q", raw, once, once, twice)
		}
	})
}
