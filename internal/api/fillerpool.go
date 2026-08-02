package api

import "context"

// Catalog-wide filler health — the Filler page's pool strip (§10 V35).
//
// This is the surface that REPLACED the mock's Coverage tab. The per-channel meter
// (`/v1/channels/{id}/filler/coverage`) answers "will THIS channel's breaks resolve"; this one
// answers "can the catalog serve the install at all, and where should a pull go".
//
// ⚠ **The per-channel numbers are not recomputed here.** `PodPreviewer.Pool` fills them by
// calling the same `Coverage` the channel route calls, once per live channel. The strip and the
// channel page therefore cannot disagree — which matters because those are precisely the two
// pages an operator compares when a channel is playing the wrong commercials. Everything this
// file adds is counting and labelling.

// PoolChannelDTO is one live channel's coverage, as the pool strip lists it.
type PoolChannelDTO struct {
	ChannelID string `json:"channelId" doc:"Channel id"`
	Name      string `json:"name" doc:"Channel name"`
	Number    int    `json:"number" doc:"Channel number"`
	Level     string `json:"level" enum:"exact,widened,audience,bumper_card" doc:"Which ladder rung this channel's breaks resolve at. bumper_card means the catalog cannot fill them and the embedded card plays."`
	Total     int    `json:"total" doc:"Eligible clips available to this channel at its widest rung"`
}

// PoolDTO is catalog-wide filler health (§10 V35).
type PoolDTO struct {
	Clips       int `json:"clips" doc:"Every clip in the catalog, of every kind"`
	Commercials int `json:"commercials" doc:"Clips that can fill a break BODY. Bumpers and station IDs bookend a pod; they cannot make one."`
	// Eligible is the headline that surprises people, so it is a field rather than something
	// a client derives: a catalog of 500 fifteen-minute compilations reads as healthy by
	// `clips` and can fill nothing.
	Eligible int `json:"eligible" doc:"Commercials that are ALSO duration-eligible under the active policy — the ones that can actually go in a break. Same gate pod assembly applies."`
	Untagged int `json:"untagged" doc:"Commercials missing a match tag. They still play, but only match broadly, so a themed channel falls back to bumpers."`
	// Channels is worst-first so a client can name "the channel to fix" without sorting, and
	// non-nil when empty so nothing has to guard before iterating.
	Channels []PoolChannelDTO `json:"channels" doc:"Live channels, WORST COVERAGE FIRST. Paused and detached channels are excluded — their breaks are not airing, so counting them as uncovered would report a problem the operator created deliberately."`
}

type fillerPoolOutput struct {
	Body PoolDTO
}

// fillerPool reports catalog-wide filler health.
//
// Read-only and member-visible, matching the per-channel coverage route: it is a count of
// material, strictly less information than the catalog listing any authenticated user can
// already read.
func (s *Server) fillerPool(ctx context.Context, _ *struct{}) (*fillerPoolOutput, error) {
	if s.pods == nil {
		return nil, errNotImplemented("Filler isn't set up", "Set up commercials and filler before checking catalog health.")
	}

	report, err := s.pods.Pool(ctx)
	if err != nil {
		return nil, err
	}

	chans := make([]PoolChannelDTO, 0, len(report.Channels))
	for _, c := range report.Channels {
		chans = append(chans, PoolChannelDTO{
			ChannelID: c.ChannelID,
			Name:      c.Name,
			Number:    c.Number,
			Level:     string(c.Report.Level),
			Total:     c.Report.Total,
		})
	}
	return &fillerPoolOutput{Body: PoolDTO{
		Clips:       report.Clips,
		Commercials: report.Commercials,
		Eligible:    report.Eligible,
		Untagged:    report.Untagged,
		Channels:    chans,
	}}, nil
}
