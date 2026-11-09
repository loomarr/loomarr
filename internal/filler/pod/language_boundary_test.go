package pod_test

import (
	"context"
	"testing"

	"github.com/loomarr/loomarr/internal/filler/language"
	"github.com/loomarr/loomarr/internal/filler/pod"
)

func spokenClip(id string, kind pod.Role, lang string) pod.Clip {
	return pod.Clip{Hash: id, Path: id + ".mp4", Name: id, Role: kind, Era: 1994,
		Audience: pod.General, DurationMs: 15_000, Language: lang}
}

// langCatalog holds English, Spanish, wordless and undetermined ads, and an English and a wordless
// station ID.
func langCatalog() []pod.Clip {
	return []pod.Clip{
		spokenClip("en1", pod.Commercial, "en"), spokenClip("en2", pod.Commercial, "en"),
		spokenClip("en3", pod.Commercial, "en"), spokenClip("en4", pod.Commercial, "en"),
		spokenClip("es1", pod.Commercial, "es"), spokenClip("es2", pod.Commercial, "es"),
		spokenClip("mute1", pod.Commercial, language.None),
		spokenClip("unknown1", pod.Commercial, language.Undetermined),
		spokenClip("en-ident", pod.StationID, "en"),
		spokenClip("mute-ident", pod.StationID, language.None),
	}
}

func langAdapter(cat []pod.Clip, household string) *pod.Adapter {
	return pod.NewAdapter(stubCatalog{clips: cat}, nil, func() pod.Policy {
		return pod.Policy{PodMax: 4, BreakDurationMs: 120_000, Language: household}
	}, discardLogger())
}

// airedOver is every clip any of 100 seeded breaks put on air.
func airedOver(t *testing.T, a *pod.Adapter, sel pod.Selection) map[string]bool {
	t.Helper()
	seen := map[string]bool{}
	for seed := range int64(100) {
		brk, err := a.Preview(context.Background(), "ch", seed, sel)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range brk.Entries {
			seen[e.Hash] = true
		}
	}
	return seen
}

func assertAired(t *testing.T, seen map[string]bool, want, never []string) {
	t.Helper()
	for _, id := range want {
		if !seen[id] {
			t.Errorf("%s never aired; aired %v", id, seen)
		}
	}
	for _, id := range never {
		if seen[id] {
			t.Errorf("%s aired", id)
		}
	}
}

// Decision 14: a Spanish channel in an English household airs Spanish and wordless clips, bookends
// included, and nothing in English or of unknown language (decision 15).
func TestLanguage_AChannelAirsOnlyItsLanguageAndWordlessClips(t *testing.T) {
	seen := airedOver(t, langAdapter(langCatalog(), "en"), pod.Selection{Language: "es"})
	assertAired(t, seen, []string{"es1", "es2", "mute1", "mute-ident"},
		[]string{"en1", "en2", "en3", "en4", "unknown1", "en-ident"})
}

// Decision 12: the channel inherits the household language, and an undetermined clip airs there.
func TestLanguage_TheHouseholdDefaultAdmitsUndeterminedClips(t *testing.T) {
	for _, sel := range []pod.Selection{{}, {Language: "en"}, {Language: "eng"}} {
		seen := airedOver(t, langAdapter(langCatalog(), "en"), sel)
		assertAired(t, seen, []string{"en1", "mute1", "unknown1", "en-ident"}, []string{"es1", "es2"})
	}
}

// Decision 12: ISO 639-2 codes, bibliographic ones included, name the same language.
func TestLanguage_ThreeLetterCodesMatch(t *testing.T) {
	seen := airedOver(t, langAdapter(langCatalog(), "eng"), pod.Selection{Language: "spa"})
	assertAired(t, seen, []string{"es1", "es2"}, []string{"en1", "unknown1"})

	cat := []pod.Clip{spokenClip("fr1", pod.Commercial, "fr"), spokenClip("en1", pod.Commercial, "en")}
	assertAired(t, airedOver(t, langAdapter(cat, "en"), pod.Selection{Language: "fre"}), []string{"fr1"}, []string{"en1"})
}

// With no language anywhere there is no boundary: the pool is what it was before #2030.
func TestLanguage_NoLanguageAnywhereAirsEverything(t *testing.T) {
	seen := airedOver(t, langAdapter(langCatalog(), ""), pod.Selection{})
	assertAired(t, seen, []string{"en1", "es1", "mute1", "unknown1"}, nil)
}

// Decision 14: when no clip speaks the channel's language the card airs, never a wrong-language ad.
func TestLanguage_AnEmptyPoolAirsTheCard(t *testing.T) {
	cat := []pod.Clip{spokenClip("en1", pod.Commercial, "en"), spokenClip("en2", pod.Commercial, "en")}
	brk, err := langAdapter(cat, "en").Preview(context.Background(), "ch", 1, pod.Selection{Language: "ja"})
	if err != nil {
		t.Fatal(err)
	}
	if brk.MatchLevel != pod.MatchBumperCard || len(brk.Entries) != 1 || !brk.Entries[0].IsFallbackCard {
		t.Fatalf("pod = %+v, want only the card", brk.Entries)
	}
}

// Decision 4: a pin skips the matching rules, the language among them.
func TestLanguage_APinSkipsTheLanguage(t *testing.T) {
	seen := airedOver(t, langAdapter(langCatalog(), "en"), pod.Selection{Language: "es", Pinned: []string{"en1"}})
	assertAired(t, seen, []string{"en1"}, []string{"en2"})
}

func TestLanguage_FitAndCoverageNameTheLanguage(t *testing.T) {
	a := langAdapter(langCatalog(), "en")
	es := pod.Selection{Language: "spa"}
	cat := langCatalog()
	byID := map[string]pod.Clip{}
	for _, c := range cat {
		byID[c.Hash] = c
	}
	for id, want := range map[string]pod.FitReason{
		"en1": pod.FitLanguage, "unknown1": pod.FitLanguage, "en-ident": pod.FitLanguage,
		"es1": "", "mute1": "",
	} {
		if got := a.FitForChannel("ch", es, byID[id]).Reason; got != want {
			t.Errorf("fit(%s) = %q, want %q", id, got, want)
		}
	}
	if got := a.FitForChannel("ch", pod.Selection{}, byID["unknown1"]).Reason; got != "" {
		t.Errorf("fit(unknown1) on the default channel = %q, want none", got)
	}

	report := a.CoverageFrom(cat, "ch", es)
	if report.Language != "es" {
		t.Errorf("coverage language = %q, want es", report.Language)
	}
	if report.UndeterminedLanguage != 1 {
		t.Errorf("coverage undetermined = %d, want 1", report.UndeterminedLanguage)
	}
	if report.Total != 3 {
		t.Errorf("coverage total = %d, want 3 (es1, es2, mute1)", report.Total)
	}
	for _, c := range report.Criteria {
		if c.Criterion == pod.CriterionLanguage && c.Clips != 3 {
			t.Errorf("language criterion = %d, want 3", c.Clips)
		}
	}
}

// A pinned clip airs whatever its language, so its fit must not report one.
func TestLanguage_APinnedClipFits(t *testing.T) {
	fit := langAdapter(langCatalog(), "en").FitForChannel("ch",
		pod.Selection{Language: "es", Pinned: []string{"en1"}}, spokenClip("en1", pod.Commercial, "en"))
	if fit.Reason != "" || !fit.Pinned {
		t.Errorf("pinned fit = %+v, want pinned with no reason", fit)
	}
}

// The undetermined count is of commercials: a bookend of unknown language is not one.
func TestLanguage_CoverageCountsUndeterminedCommercialsOnly(t *testing.T) {
	cat := append(langCatalog(), spokenClip("unknown-ident", pod.StationID, language.Undetermined))
	if got := langAdapter(cat, "en").CoverageFrom(cat, "ch", pod.Selection{}).UndeterminedLanguage; got != 1 {
		t.Errorf("undetermined = %d, want 1", got)
	}
}
