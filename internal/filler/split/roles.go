package split

// VisionKeyframes is how many stills one pass samples per clip (§10 V44). A commercial's brand
// card, its B&W transfer, and its end slate can each fall in a different part of the runtime, so
// one frame is not enough signal. These are bounded near-full-resolution semantic frames, not the
// 320px UI preview: early, middle, late, and closing-card windows remain a small fixed request.
const VisionKeyframes = 4

// CertifiedFillerRole reports whether role is one a kept, certified filler segment may carry.
func CertifiedFillerRole(role Role) bool {
	switch role {
	case RoleCommercial, RolePromo, RoleBumper, RoleStationID, RolePSA, RoleTrailer, RoleInterstitial:
		return true
	default:
		return false
	}
}
