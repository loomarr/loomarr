package api

import "testing"

// What an operator pastes into "Add a source" is not consistent, and the three shapes below are
// all things people really paste. Anything else must be REFUSED rather than guessed at: a row
// built from an unparseable string is a source that can never fetch and never explains why.
func TestArchiveIdentifier(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"classic_tv_commercials", "classic_tv_commercials"},
		{"https://archive.org/details/classic_tv_commercials", "classic_tv_commercials"},
		{"http://archive.org/details/classic_tv_commercials/", "classic_tv_commercials"},
		{"archive.org/details/classic_tv_commercials?sort=date", "classic_tv_commercials"},
		{"/details/classic_tv_commercials", "classic_tv_commercials"},
		{"  classic_tv_commercials  ", "classic_tv_commercials"},

		// Refused. Each of these would otherwise become a row that looks registered.
		{"", ""},
		{"https://example.com/some/page", ""},
		{"not an id", ""},
		{"https://archive.org/", ""},
		{"archive.org/details/", ""},
	} {
		if got := archiveIdentifier(tc.in); got != tc.want {
			t.Errorf("archiveIdentifier(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
