package filler_test

import (
	"testing"

	"github.com/loomarr/loomarr/internal/filler"
)

// The language gate's decision logic (§10 V40). These are pure functions on purpose: the two
// backends differ entirely in how they arrive at a code, and identically in what the code MEANS.

// ⚠ Comparing raw codes is the obvious implementation and it fails on real data: whisper answers
// `en`, a hosted model may answer `en-US` or `English`, and an operator may type `EN` into the
// setting. Any mismatch means the gate rejects every English clip in the catalog.
func TestNormalizeLanguage(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"en", "en"},
		{"EN", "en"},
		{"  en  ", "en"},
		{"en-US", "en"},   // region subtag
		{"pt_BR", "pt"},   // underscore form
		{"zh-Hans", "zh"}, // script subtag
		{"English", "en"}, // a model answering in prose
		{"eng", "en"},     // ISO 639-2
		{"spa", "es"},
		{"", ""},
		// ⚠ An unrecognised value stays as-is rather than being coerced. It then matches nothing,
		// which keeps the clip — the honest outcome for an answer we do not understand.
		{"klingon", "klingon"},
	} {
		if got := filler.NormalizeLanguage(tc.in); got != tc.want {
			t.Errorf("NormalizeLanguage(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// ⚠ **THE polarity rule.** The gate rejects only on a POSITIVE mismatch; every uncertain state
// keeps the clip. The two mistakes are not symmetric: a foreign advert that slips through plays
// once and annoys, while a wrongly-rejected clip is deleted and the operator is never told which.
func TestLanguageRejects_OnlyOnAPositiveMismatch(t *testing.T) {
	for _, tc := range []struct {
		name           string
		detected, want string
		reject         bool
	}{
		{"a confident mismatch is the whole point", "es", "en", true},
		{"a match is kept", "en", "en", false},
		{"a match through normalisation is kept", "en-US", "EN", false},

		// Every one of these is a state where the gate must NOT fire.
		{"not yet checked", filler.LangUndetermined, "en", false},
		{"no speech to judge", filler.LangNone, "en", false},
		{"gate disabled by an empty setting", "es", "", false},
		{"an answer we could not parse", "", "en", false},

		// #2030 decision 13: the gate admits a set, the channels' commercial languages.
		{"a language some channel airs is kept", "es", "en,es", false},
		{"a language no channel airs is refused", "fr", "en,es", true},
		{"silence is kept against a set too", filler.LangNone, "en,es", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := filler.LanguageRejects(tc.detected, tc.want); got != tc.reject {
				t.Errorf("LanguageRejects(%q, %q) = %v, want %v", tc.detected, tc.want, got, tc.reject)
			}
		})
	}
}

// #2030 decision 13: intake admits every language some channel airs. A channel's commercial language
// is its own, else the household's, so adding a channel in a new language opens the gate to it with
// no settings change, and removing the last one closes it again.
func TestIntakeLanguages_IsTheUnionOfTheChannelsLanguages(t *testing.T) {
	for _, tc := range []struct {
		name      string
		household string
		channels  []string
		want      string
	}{
		{"no channels yet: the household's language", "en", nil, "en"},
		{"no channels and no household language: the gate is off", "", nil, ""},
		{"a channel in a new language joins the household's", "en", []string{"", "es"}, "en,es"},
		{"the household's language only counts through a channel using it", "en", []string{"es", "ja"}, "es,ja"},
		// A channel with no language airs every language, so intake cannot refuse any.
		{"a channel with no language turns the gate off", "", []string{"", "es"}, ""},
		{"codes are mapped and counted once", "eng", []string{"spa", "ES", ""}, "en,es"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := filler.IntakeLanguages(tc.household, tc.channels); got != tc.want {
				t.Errorf("IntakeLanguages(%q, %q) = %q, want %q", tc.household, tc.channels, got, tc.want)
			}
		})
	}
}

// ⚠ **Silence must never reject**, stated as its own test because it is the rule most likely to be
// "simplified" away. A wordless visual spot has no language, and those are often the best filler —
// so `none` is a final answer that KEEPS, not a missing answer.
func TestLanguageRejects_SilenceIsNeverGroundsForRejection(t *testing.T) {
	for _, want := range []string{"en", "es", "fr", "ja"} {
		if filler.LanguageRejects(filler.LangNone, want) {
			t.Errorf("a clip with no speech was rejected against %q — wordless adverts are filler, "+
				"not foreign-language clips", want)
		}
	}
}

// The sampled window. A normal commercial is inspected in full: live evidence from a 30-second
// Spanish sandwich-chain spot showed that nearby ten-second cuts alternated between English and
// Spanish while its complete speech was correctly classified as Spanish.
func TestLanguageSpan(t *testing.T) {
	t.Run("takes all of a normal spot", func(t *testing.T) {
		start, end := filler.LanguageSpan(30_000)
		if start != 0 || end != 30_000 {
			t.Errorf("span = [%d,%d), want the complete [0,30000) commercial", start, end)
		}
		if end-start != filler.LanguageSampleMs {
			t.Errorf("span = %dms, want %dms", end-start, filler.LanguageSampleMs)
		}
	})

	// ⚠ A clip SHORTER than the window is inspected in full. Asking past a 6s bumper would produce
	// an empty tail and make the result depend on audio that does not exist.
	t.Run("takes all of a clip shorter than the window", func(t *testing.T) {
		start, end := filler.LanguageSpan(6_000)
		if start != 0 || end != 6_000 {
			t.Errorf("span = [%d,%d), want the whole 6s clip", start, end)
		}
	})

	// The window never runs past the end.
	t.Run("never runs past the end", func(t *testing.T) {
		const dur = 10_500
		_, end := filler.LanguageSpan(dur)
		if end > dur {
			t.Errorf("span ends at %dms, past the clip's %dms", end, dur)
		}
	})

	// Unprobed is a real state — ask for the window and let the backend take what exists.
	t.Run("still asks for a window when the duration is unknown", func(t *testing.T) {
		start, end := filler.LanguageSpan(0)
		if end <= start {
			t.Errorf("span = [%d,%d), want a usable window", start, end)
		}
	})
}
