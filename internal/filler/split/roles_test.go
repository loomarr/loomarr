package split

import "testing"

func TestLookRole(t *testing.T) {
	for answer, want := range map[string]Role{
		"commercial":         RoleCommercial,
		" Political ":        RoleCommercial, // a campaign ad is a commercial to the split (#4418)
		"news":               "news",         // may be a newscast inside a reel: not filler
		"programme_fragment": RoleProgrammeFragment,
		"":                   "",
	} {
		got := LookRole(answer)
		if got != want {
			t.Errorf("LookRole(%q) = %q, want %q", answer, got, want)
		}
		if answer == "news" && CertifiedFillerRole(got) {
			t.Error("a news answer became a filler role")
		}
	}
	if !CertifiedFillerRole(LookRole("political")) {
		t.Error("a campaign ad is refused as not break material")
	}
}
