package playoutcert

import (
	"context"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/testkit/playoutcertfixture"
)

func TestManifestDigestBindsChannelBoundariesOrderAndRoles(t *testing.T) {
	channelsA := []Channel{{ID: "alpha", Roles: []string{"copy"}}, {ID: "audio_aac"}}
	channelsB := []Channel{{ID: "alpha"}, {ID: "copy", Roles: []string{"audio_aac"}}}
	for index := 0; index < 98; index++ {
		channel := Channel{ID: "channel-" + string(rune('a'+index/26)) + string(rune('a'+index%26))}
		channelsA = append(channelsA, channel)
		channelsB = append(channelsB, channel)
	}
	for _, channels := range [][]Channel{channelsA, channelsB} {
		config := Config{BaseURL: "http://127.0.0.1:8080", AdminBearer: "admin", DeviceToken: "device", Channels: channels, Certify: true}
		if err := config.Validate(); err != nil {
			t.Fatalf("valid 100-channel declaration rejected: %v", err)
		}
	}
	if manifestDigest(channelsA) == manifestDigest(channelsB) {
		t.Fatal("different channel/role declarations share a manifest digest")
	}
	if manifestDigest(channelsA) != manifestDigest(append([]Channel(nil), channelsA...)) {
		t.Fatal("digest is not deterministic")
	}
	reordered := append([]Channel(nil), channelsA...)
	reordered[0], reordered[1] = reordered[1], reordered[0]
	if manifestDigest(channelsA) == manifestDigest(reordered) {
		t.Fatal("channel order does not affect digest")
	}
	changedRoles := append([]Channel(nil), channelsA...)
	changedRoles[0].Roles = []string{"prepared"}
	if manifestDigest(channelsA) == manifestDigest(changedRoles) {
		t.Fatal("roles do not affect digest")
	}
}

func TestTargetRevisionRequiresFullGitRevision(t *testing.T) {
	for _, tc := range []struct {
		revision string
		valid    bool
	}{
		{revision: strings.Repeat("a", 40), valid: true},
		{revision: strings.Repeat("0", 40), valid: true},
		{revision: "0123456"},
		{revision: strings.Repeat("a", 39)},
		{revision: strings.Repeat("a", 41)},
		{revision: strings.Repeat("g", 40)},
	} {
		t.Run(tc.revision, func(t *testing.T) {
			fixture := playoutcertfixture.New(t, 1)
			fixture.Revision = tc.revision
			endpoint, err := newEndpoint(Config{BaseURL: fixture.Server.URL, Client: fixture.Server.Client(), AdminBearer: fixture.Admin})
			if err != nil {
				t.Fatal(err)
			}
			_, _, err = endpoint.target(context.Background(), 100, "digest")
			if (err == nil) != tc.valid {
				t.Fatalf("target revision accepted=%t, error=%v", err == nil, err)
			}
		})
	}
}

func TestRunRejectsMalformedCohortIdentityBeforeTargetAccess(t *testing.T) {
	for _, digest := range []string{"private path", strings.Repeat("a", 63), strings.Repeat("A", 64), strings.Repeat("g", 64)} {
		source := &playoutcertfixture.ProgrammeEvidence[ProgrammeEvidence]{ManifestSHA256: digest}
		_, err := Run(t.Context(), Config{BaseURL: "http://127.0.0.1:1", AdminBearer: "admin", DeviceToken: "device", Channels: []Channel{{ID: "channel"}}, ProgrammeEvidence: source})
		if err == nil || err.Error() != "invalid operator cohort identity" {
			t.Fatalf("invalid identity was not rejected before target access: %v", err)
		}
	}
}
