package clips

import "strings"

// PlacementForRole derives scheduling placement without inventing a descriptive role. An enrolled
// unknown is ordinary break-body filler; composites remain containers and can never be scheduled.
func PlacementForRole(kind Role, enrolled, composite bool) Placement {
	if composite || !enrolled {
		return PlacementNotPlayable
	}
	switch kind {
	case Bumper, StationID:
		return PlacementBookend
	case Commercial, PSA, Trailer, Interstitial, Political, News, Unclassified:
		return PlacementBreakBody
	default:
		return PlacementNotPlayable
	}
}

// AudienceFromString parses an Audience; unknown/empty → "" (untagged).
func AudienceFromString(s string) Audience {
	switch Audience(strings.ToLower(s)) {
	case Kids, Family, General, LateNight:
		return Audience(strings.ToLower(s))
	default:
		return ""
	}
}
