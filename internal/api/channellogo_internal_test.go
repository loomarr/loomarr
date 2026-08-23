package api

import (
	"fmt"
	"testing"

	"github.com/loomarr/loomarr/internal/images"
)

// imageHashFromLogo is the seam between an operator-controlled string and an image-store lookup
// key, which is why it validates rather than merely extracts. `PATCH /v1/channels/{id}` accepts
// any `logo` value, so whatever this returns is attacker-influenced by construction.
func TestImageHashFromLogo(t *testing.T) {
	const hash = "9f2b1c4e8a7d65039f2b1c4e8a7d65039f2b1c4e8a7d65039f2b1c4e8a7d6503"

	for _, tc := range []struct {
		name string
		logo string
		want string
	}{
		{"a relative rendition URL", "/v1/images/" + hash + "/w500.jpg", hash},
		{"an absolute rendition URL", "https://loomarr.example/v1/images/" + hash + "/w92.webp", hash},
		{"the bare record URL", "/v1/images/" + hash, hash},
		{"a record URL with a query", "/v1/images/" + hash + "?x=1", hash},

		// External logos are the supported non-service case, not a legacy state — pasting a URL
		// is a documented way to set a channel icon, so these must resolve to "" and fall back.
		{"a TMDB poster", "https://image.tmdb.org/t/p/w500/abc.jpg", ""},
		{"empty", "", ""},

		// ⚠ The security cases. A bare "take the segment after /v1/images/" would forward any of
		// these to the store as a lookup key.
		{"path traversal", "/v1/images/../../etc/passwd", ""},
		{"not hex", "/v1/images/zzzz1c4e8a7d65039f2b1c4e8a7d65039f2b1c4e8a7d65039f2b1c4e8a7d6503/w500.jpg", ""},
		{"too short", "/v1/images/9f2b1c4e/w500.jpg", ""},
		{"too long", "/v1/images/" + hash + "aa/w500.jpg", ""},
		// Uppercase is refused rather than lowered: the service only ever emits lowercase hex, so
		// an uppercase value did not come from us and should take the external path.
		{"uppercase hex", "/v1/images/9F2B1C4E8A7D65039F2B1C4E8A7D65039F2B1C4E8A7D65039F2B1C4E8A7D6503", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := imageHashFromLogo(tc.logo); got != tc.want {
				t.Errorf("imageHashFromLogo(%q) = %q, want %q", tc.logo, got, tc.want)
			}
		})
	}
}

func TestBrowserLogoURLUsesThePageOriginForServiceImages(t *testing.T) {
	const hash = "9f2b1c4e8a7d65039f2b1c4e8a7d65039f2b1c4e8a7d65039f2b1c4e8a7d6503"
	pathFor := func(hash string, width int, format images.Format) string {
		return fmt.Sprintf("/v1/images/%s/w%d.%s", hash, width, format.Ext())
	}

	got := browserLogoURL("http://machine-client-only.invalid:8080/v1/images/"+hash+"/w500.jpg", pathFor)
	want := "/v1/images/" + hash + "/w500.jpg"
	if got != want {
		t.Errorf("browserLogoURL(service image) = %q, want %q", got, want)
	}

	external := "https://operator.example/icon.jpg"
	if got := browserLogoURL(external, pathFor); got != external {
		t.Errorf("browserLogoURL(external image) = %q, want the operator URL preserved", got)
	}
}
