package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/store"
)

// GET /v1/channels/{id}/formats — the formats a channel airs (#1512 G10), read-only.
//
// Every channel airs the 1080p SDR H.264 baseline. The premium HEVC format (4K SDR or 4K HDR10) is
// derived from the lineup's inventory facts, top resolution and dynamic range independently, then
// gated by what this host can produce. Both the lineup's answer and the host's are returned, so a
// dropped premium says why.

func (s *Server) registerChannelFormats(api huma.API) {
	huma.Register(api, withRole(huma.Operation{
		OperationID: "channel-formats", Method: http.MethodGet, Path: "/v1/channels/{id}/formats",
		Summary: "Output formats this channel airs",
		Description: "Any signed-in user. Every channel airs a 1080p SDR H.264 baseline. When its lineup has 4K " +
			"titles it also airs one premium HEVC format: 4K HDR10 if any title is HDR, otherwise 4K SDR. Derived " +
			"from Loomarr's media inventory, never a setting. `lineupPremium` is what the lineup warrants; `premium` " +
			"is what this server produces, and `premiumDropped` says why they differ (software-only hosts never " +
			"produce a premium).",
		Tags: []string{"channels"},
	}, RoleMember), s.channelFormats)
}

// FormatDTO is one output format.
type FormatDTO struct {
	Class        string `json:"class" enum:"1080p-h264-sdr,4k-hevc-sdr,4k-hevc-hdr" doc:"Format class; the premium names are the capacity classes the resource budget measures"`
	Codec        string `json:"codec" enum:"h264,hevc" doc:"Video codec"`
	Width        int    `json:"width" doc:"Frame width in pixels"`
	Height       int    `json:"height" doc:"Frame height in pixels"`
	DynamicRange string `json:"dynamicRange" enum:"sdr,hdr10" doc:"sdr (BT.709) or hdr10 (BT.2020 PQ with static channel-level metadata)"`
}

type channelFormatsOutput struct {
	Body struct {
		Baseline       FormatDTO  `json:"baseline" doc:"The 1080p SDR H.264 baseline every channel airs"`
		Premium        *FormatDTO `json:"premium" doc:"The premium format this server airs for the channel; null when none"`
		LineupPremium  string     `json:"lineupPremium" enum:",4k-hevc-sdr,4k-hevc-hdr" doc:"The premium the lineup warrants before host limits; empty when none"`
		PremiumDropped string     `json:"premiumDropped,omitempty" doc:"Why this server does not air the premium the lineup warrants"`
		Titles         int        `json:"titles" doc:"Distinct programmes in the lineup"`
		MeasuredTitles int        `json:"measuredTitles" doc:"Programmes with inventory stream facts; unmeasured ones cannot warrant a premium"`
		UHDTitles      int        `json:"uhdTitles" doc:"Measured programmes larger than 2560x1440"`
		HDRTitles      int        `json:"hdrTitles" doc:"Measured programmes with an HDR transfer (PQ or HLG)"`
	}
}

func formatDTO(c playout.FormatClass) FormatDTO {
	spec, _ := c.Spec()
	return FormatDTO{Class: string(c), Codec: spec.Codec, Width: spec.Width, Height: spec.Height, DynamicRange: spec.DynamicRange}
}

func (s *Server) channelFormats(ctx context.Context, in *channelIDInput) (*channelFormatsOutput, error) {
	if _, err := s.store.GetChannel(ctx, in.ID); errors.Is(err, store.ErrNotFound) {
		return nil, errNotFound("Channel not found", "That channel doesn't exist — it may have been removed.")
	} else if err != nil {
		return nil, err
	}
	out := &channelFormatsOutput{}
	out.Body.Baseline = formatDTO(playout.FormatBaseline)
	// No resolver (no playout wired): baseline only, which is what such a server airs.
	if s.playoutResolver == nil {
		return out, nil
	}
	lineup, err := s.playoutResolver.LineupFormats(ctx, in.ID)
	if err != nil {
		return nil, apiErrWithCause(http.StatusServiceUnavailable, "Lineup unavailable",
			"Loomarr couldn't read this channel's lineup. Try again in a moment.", err)
	}
	for _, f := range lineup {
		out.Body.Titles++
		if f.VideoCodec == "" && f.Width == 0 {
			continue
		}
		out.Body.MeasuredTitles++
		if playout.Is4K(f) {
			out.Body.UHDTitles++
		}
		if f.HDR() {
			out.Body.HDRTitles++
		}
	}
	derived := playout.DeriveChannelFormats(lineup)
	out.Body.LineupPremium = string(derived.Premium)
	host := playout.HostFor(s.playoutResolver.Profile(ctx, 0).Encoder,
		s.playoutTonemap != nil && s.playoutTonemap(), s.gpuTonemapFilters())
	aired, why := derived.OnHost(host)
	out.Body.PremiumDropped = why
	if aired.Premium != "" {
		p := formatDTO(aired.Premium)
		out.Body.Premium = &p
	}
	return out, nil
}

func (s *Server) gpuTonemapFilters() playout.GPUFilters {
	if s.playoutGPUTonemap == nil {
		return playout.GPUFilters{}
	}
	return s.playoutGPUTonemap()
}
