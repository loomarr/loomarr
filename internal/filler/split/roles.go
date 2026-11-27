package split

import "strings"

// VisionKeyframes is how many stills one pass samples per clip (§10 V44). A commercial's brand
// card, its B&W transfer, and its end slate can each fall in a different part of the runtime, so
// one frame is not enough signal. These are bounded near-full-resolution semantic frames, not the
// 320px UI preview: early, middle, late, and closing-card windows remain a small fixed request.
const VisionKeyframes = 4

// WholeCutKeyframes is how many stills the split look samples across one cut. More than a single
// clip's look, because this one must also see a change of spot inside the cut: two 15 s ads joined
// by a hard cut put two or three of these frames in each (#4409).
const WholeCutKeyframes = 8

// CertifiedFillerRole reports whether role is one a kept, certified filler segment may carry.
func CertifiedFillerRole(role Role) bool {
	switch role {
	case RoleCommercial, RolePromo, RoleBumper, RoleStationID, RolePSA, RoleTrailer, RoleInterstitial:
		return true
	default:
		return false
	}
}

// LookRole reads the role the split look gave a cut (#4418). The look shares the clip vision
// prompt, and a model that recognises a campaign ad answers `political`, a clip kind (#2037) that is
// a kind of commercial rather than a split role. Measured: 6 of 20 whole spots were refused for it.
// The cut-out clip is classified again, and only channels that accept political ads air it.
//
// `news` is NOT mapped: inside a reel it may be a newscast, which is not filler.
func LookRole(answer string) Role {
	role := Role(strings.ToLower(strings.TrimSpace(answer)))
	if role == "political" {
		return RoleCommercial
	}
	return role
}

// WholeCut is the look at a whole cut (#4409): whether its frames show one complete piece of filler.
type WholeCut string

const (
	// WholeOne is one complete ad, promo, ident or spot, from its start to its end.
	WholeOne WholeCut = "one"
	// WholePart is part of one: it starts or ends mid-spot.
	WholePart WholeCut = "part"
	// WholeSeveral is more than one: a change of advertiser, product or spot inside the cut.
	WholeSeveral WholeCut = "several"
	// WholeUnsure is a look that could not tell, or an answer that could not be read.
	WholeUnsure WholeCut = "unsure"
)

// ParseWholeCut reads a model's answer. Anything but a known value is WholeUnsure: an answer that
// cannot be read is a doubt, and doubt never cuts.
func ParseWholeCut(s string) WholeCut {
	switch w := WholeCut(strings.ToLower(strings.TrimSpace(s))); w {
	case WholeOne, WholePart, WholeSeveral:
		return w
	default:
		return WholeUnsure
	}
}
