package app

import "testing"

// Internal playout (the default backend) needs server.public_url so a media client can dial
// its HLS segments; unset, channels appear in the guide and fail at tune time. The boot path
// warns on that condition (beta-readiness D-4), and this pins the predicate that drives it.
func TestInternalPlayoutNeedsPublicURL(t *testing.T) {
	cases := []struct {
		name      string
		publicURL string
		want      bool
	}{
		{"empty warns", "", true},
		{"whitespace-only warns", "   ", true},
		{"set does not warn", "http://loomarr.home:8080", false},
		{"set with surrounding space does not warn", "  http://loomarr.home:8080  ", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := internalPlayoutNeedsPublicURL(tc.publicURL); got != tc.want {
				t.Fatalf("internalPlayoutNeedsPublicURL(%q) = %v, want %v", tc.publicURL, got, tc.want)
			}
		})
	}
}
