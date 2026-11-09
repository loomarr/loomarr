package watermark

import "testing"

// The Plate's callsign is broadcast-short, deterministic from the channel name (the issue's LLM
// pick is off): an acronym the name already has, else a decade or number, else the initials of
// its significant words, else its one word.
func TestDeriveCallsign(t *testing.T) {
	for name, want := range map[string]string{
		"Saturday Cartoons":                  "SC",
		"Retro Cartoons":                     "RC",
		"QFUN Sitcoms":                       "QFUN",
		"80s Horror":                         "80s",
		"The Sci-Fi Vault":                   "SV",
		"A Midnight Movies":                  "MM",
		"Movies & More":                      "MM",
		"Friday Night Horror Movie Marathon": "FNHM",
		"Ciné Club":                          "CC",
		"RETRO CARTOONS":                     "RC",
		"Westerns":                           "WESTERNS",
		"Supercalifragilistic":               "SUPERCAL",
		"The":                                "THE",
		"!":                                  "CH4",
	} {
		if got := DeriveCallsign(name, 4); got != want {
			t.Errorf("%q: %q, want %q", name, got, want)
		}
	}
}
