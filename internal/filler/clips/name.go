package clips

import (
	"strconv"
	"strings"
)

// RoleFromName records an explicit filler-role token from the filename convention (§10 — the
// cheapest tagging tier). Absence is Unclassified, never a placement default: acquisition and
// unrelated grounding do not prove that unknown bytes are a commercial.
func RoleFromName(name string) Role {
	tokens := strings.FieldsFunc(strings.ToLower(name), func(r rune) bool {
		return r < 'a' || r > 'z'
	})
	for index, token := range tokens {
		switch token {
		case "bumper", "bumpers":
			return Bumper
		case "ident", "idents":
			return StationID
		case "station":
			if index+1 < len(tokens) && (tokens[index+1] == "id" || tokens[index+1] == "ident") {
				return StationID
			}
		case "psa", "psas":
			return PSA
		case "trailer", "trailers":
			return Trailer
		case "interstitial", "interstitials":
			return Interstitial
		case "commercial", "commercials", "advert", "adverts", "ad", "ads":
			return Commercial
		}
	}
	return Unclassified
}

// EraFromName best-effort extracts a 4-digit year (1930–2035) from a filename
// ("Total-Cereal 1985.mp4" → 1985), an initial era tag before AI refinement.
// Returns 0 if none.
func EraFromName(name string) int {
	for i := 0; i+4 <= len(name); i++ {
		if y, err := strconv.Atoi(name[i : i+4]); err == nil && y >= 1930 && y <= 2035 {
			return y
		}
	}
	return 0
}
