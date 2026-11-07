package api

import (
	"log/slog"
	"sort"
	"testing"

	"github.com/loomarr/loomarr/internal/people/auth"
)

// Every request field that SETS a password declares auth.MinPasswordLen as its minLength, so the
// spec carries the one password rule to every client and the generated web client reads it from
// there instead of hard-coding 8. The walk covers every password-format property in the spec, so
// a new password route that forgets the tag fails here rather than accepting a one-character
// password at the API while the web form refuses it.
//
// Exempt are the fields that only PROVE a secret already held: the sign-in password, the current
// password a self-service change checks, and the single-use grants. Their minimum is whatever the
// secret already is.
func TestPasswordRoutesDeclareTheMinimum(t *testing.T) {
	_, humaAPI := schemaOnlyAPI(slog.New(slog.DiscardHandler))
	exempt := map[string]bool{
		"LoginInputBody.password":               true,
		"ChangePasswordInputBody.current":       true,
		"InvitationBearerInputBody.grant":       true,
		"RedeemInvitationInputBody.grant":       true,
		"PasswordRecoveryBearerInputBody.grant": true,
		"RedeemPasswordRecoveryInputBody.grant": true,
	}

	var setting, seenExempt []string
	for name, schema := range humaAPI.OpenAPI().Components.Schemas.Map() {
		for prop, field := range schema.Properties {
			if field.Format != "password" {
				continue
			}
			key := name + "." + prop
			if exempt[key] {
				seenExempt = append(seenExempt, key)
				continue
			}
			setting = append(setting, key)
			if field.MinLength == nil || *field.MinLength != auth.MinPasswordLen {
				t.Errorf("%s sets a password but declares minLength %v, want %d (auth.MinPasswordLen)",
					key, field.MinLength, auth.MinPasswordLen)
			}
		}
	}
	sort.Strings(setting)
	sort.Strings(seenExempt)

	// A walk that finds nothing guards nothing: bootstrap, invitation, recovery, change, create
	// and reset each set one.
	if len(setting) < 6 {
		t.Fatalf("found %d password-setting fields %v; the walk stopped finding the routes", len(setting), setting)
	}
	for key := range exempt {
		found := false
		for _, s := range seenExempt {
			found = found || s == key
		}
		if !found {
			t.Errorf("exemption %s matches no password field; remove it", key)
		}
	}
}
