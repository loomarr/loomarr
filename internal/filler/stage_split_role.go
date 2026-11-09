package filler

import (
	"slices"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/filler/split"
	"github.com/loomarr/loomarr/internal/platform/llm"
)

func structureRoleEvidenceFromVision(source split.SourceAsset, segment split.Segment, promptVersion, prompt string, frames [][]byte, response llm.Response, out visionOutput, assessedAt time.Time) (*split.RoleEvidence, error) {
	role := split.Role(strings.TrimSpace(out.Role))
	if !split.ValidRole(role) || strings.TrimSpace(out.RoleReason) == "" {
		return nil, nil
	}
	var charge *split.RoleCharge
	if response.Attribution.Charge != nil {
		charge = &split.RoleCharge{Amount: response.Attribution.Charge.Amount, Currency: response.Attribution.Charge.Currency}
	}
	tokens := response.Attribution.Tokens
	evidence, err := split.NewRoleEvidence(split.RoleEvidenceInput{
		Source: source, StartMs: segment.StartMs, EndMs: segment.EndMs, Role: role, Reason: out.RoleReason,
		Frames: frames, PromptVersion: promptVersion, Prompt: prompt, Response: response.Content,
		RequestedProvider: response.Attribution.RequestedProvider, ResolvedProvider: response.Attribution.ResolvedProvider,
		RequestedModel: response.Attribution.RequestedModel, ResolvedModel: response.Attribution.ResolvedModel,
		Modalities: slices.Clone(response.Attribution.Modalities),
		Tokens: split.RoleTokenUsage{
			Prompt: tokens.Prompt, Completion: tokens.Completion, Reasoning: tokens.Reasoning,
			Cached: tokens.Cached, CacheWrite: tokens.CacheWrite, Image: tokens.Image, Audio: tokens.Audio, Video: tokens.Video,
		},
		Charge: charge, LatencyMs: response.Attribution.Latency.Milliseconds(), Attempts: response.Attribution.Attempts,
		GenerationID: response.Attribution.GenerationID, AssessedAt: assessedAt,
	})
	if err != nil {
		return nil, err
	}
	return &evidence, nil
}

// reassessProposalStructure adds the proposal's retained per-span role evidence to the immutable
// detector facts. Evidence for a span that does not exactly match the deterministic plan remains
// auditable but cannot become a role claim.
func reassessProposalStructure(proposal SplitProposal, assessedAt time.Time) (split.Assessment, error) {
	return split.Reassess(proposal.Source, proposal.Structure, proposal.StructureDecision, proposal.Segments, assessedAt)
}
