package scan

import (
	"testing"

	mp "github.com/loomarr/loomarr/internal/library/metadataprovider"
)

// TestAnimeReleaseNumbers pins what a fansub's name numbers where the naming matrix can't see it:
// an absolute number and a season 0 with no episode both read there as "S?E?".
func TestAnimeReleaseNumbers(t *testing.T) {
	for _, c := range []struct {
		name string
		show string
		want mp.Numbering
	}{
		{"[Grp] Tsuki no Kawa - 19 [1080p].mkv", "Tsuki no Kawa", mp.Numbering{Absolute: 19}},
		{"[Grp] Tsuki no Kawa - 19.5 [1080p].mkv", "Tsuki no Kawa", mp.Numbering{Season: 0}},
		{"[Grp] Tsuki no Kawa - 06.5v2 [1080p].mkv", "Tsuki no Kawa", mp.Numbering{Season: 0}},
		{"[Grp] Tsuki no Kawa - Recap - 06.5.mkv", "Tsuki no Kawa", mp.Numbering{Season: 0}},
	} {
		show, n, ok := looseAnime(c.name)
		if !ok || show != c.show || n != c.want {
			t.Errorf("looseAnime(%q) = %q %+v %v, want %q %+v", c.name, show, n, ok, c.show, c.want)
		}
	}
}
