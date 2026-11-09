package pod

import "github.com/loomarr/loomarr/internal/filler/clips"

// The clip catalog's value types are defined in internal/filler/clips. Pod assembly names them
// here, as aliases of the same types, as the filler root does, so break logic reads in its own
// terms. Their documentation lives on the definitions.
type (
	Clip     = clips.Clip
	Role     = clips.Role
	Audience = clips.Audience
)

const (
	Unclassified = clips.Unclassified
	Commercial   = clips.Commercial
	Bumper       = clips.Bumper
	StationID    = clips.StationID
	PSA          = clips.PSA
	Trailer      = clips.Trailer
	Interstitial = clips.Interstitial
	Political    = clips.Political
	News         = clips.News

	PlacementNotPlayable = clips.PlacementNotPlayable
	PlacementBreakBody   = clips.PlacementBreakBody

	Kids      = clips.Kids
	Family    = clips.Family
	General   = clips.General
	LateNight = clips.LateNight
)
