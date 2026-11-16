package filler

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/loomarr/loomarr/internal/api/httpcore"
	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/filler/language"
	"github.com/loomarr/loomarr/internal/filler/operator"
	"github.com/loomarr/loomarr/internal/filler/split"
	"github.com/loomarr/loomarr/internal/platform/store"
)

type splitFillerInput struct {
	Body struct {
		// Hash identifies the compilation clip (its content hash, V38c). ⚠ In the BODY (§10 V45a):
		// the wire identity is the hash. This is ALSO what Propose needs — it is keyed `WHERE hash = ?`
		// — so the hash passes straight through with no path lookup.
		Hash string `json:"hash" doc:"The compilation clip's content hash (its identity, §10)"`
	}
}
type splitFillerOutput struct {
	Body struct {
		JobID string `json:"jobId" doc:"Detection operation id — read /v1/filler/split-operations/{jobId}; filler_split events only accelerate progress"`
	}
}

// splitFiller starts detection on a compilation clip. It answers immediately:
// a full-decode detection pass takes minutes, so the job id's progress rides
// the SSE bus and the proposal is read back when the terminal frame lands.
func (s *Handlers) splitFiller(ctx context.Context, in *splitFillerInput) (*splitFillerOutput, error) {
	if s.filler == nil {
		return nil, httpcore.NotImplemented("Filler isn't set up",
			"Enable filler in Settings before splitting a compilation.")
	}
	if in.Body.Hash == "" {
		return nil, httpcore.Unprocessable("Missing clip", "A split must name the compilation clip by its hash.")
	}
	jobID, err := s.filler.Split(ctx, in.Body.Hash)
	if errors.Is(err, store.ErrNotFound) {
		return nil, httpcore.NotFound("Clip not found", "That filler clip doesn't exist — it may have been removed by a catalog sync.")
	}
	if errors.Is(err, operator.ErrSplitUnavailable) {
		return nil, httpcore.Conflict("Splitting isn't set up",
			"Splitting needs the filler drop-folder (filler.dir). Set it in Settings → Filler.")
	}
	if err != nil {
		return nil, err
	}
	out := &splitFillerOutput{}
	out.Body.JobID = jobID
	return out, nil
}

type getFillerSplitInput struct {
	ProposalID string `path:"proposalId"`
}

// SplitReviewSegmentDTO is the review projection of one detected span. The embedded cut fields
// remain the one generated contract used by confirmation; Artwork is server-authored presentation
// evidence and is never accepted back on the write path.
type SplitReviewSegmentDTO struct {
	split.Segment
	Artwork *httpcore.ImageDTO `json:"artwork,omitempty" doc:"Representative still from this exact proposed span; absent when extraction was unavailable"`
}

// SplitReviewProposalDTO keeps detector state private while giving the review surface complete
// image records in one read. Hashes and filesystem paths are not a frontend rendering contract.
type SplitReviewProposalDTO struct {
	ID        string                  `json:"id"`
	ClipHash  string                  `json:"clipHash"`
	CreatedAt time.Time               `json:"createdAt"`
	Segments  []SplitReviewSegmentDTO `json:"segments"`
	// CheckedLanguages is the set this proposal was checked against; empty when it was not checked.
	CheckedLanguages []string `json:"checkedLanguages"`
	// CurrentLanguages is what intake admits now, the channels' commercial languages (#2030
	// decision 13); empty admits every language. Review offers a recheck when the two differ. When
	// the channels cannot be read it repeats CheckedLanguages, so no recheck is offered on a guess.
	CurrentLanguages   []string                        `json:"currentLanguages"`
	LanguageExclusions []filler.SplitLanguageExclusion `json:"languageExclusions,omitempty"`
}

type getFillerSplitOutput struct {
	Body SplitReviewProposalDTO
}

// getFillerSplit reads one proposal — the review surface's source of truth.
// Read straight from the store, like list/patch: no service indirection needed.
func (s *Handlers) getFillerSplit(ctx context.Context, in *getFillerSplitInput) (*getFillerSplitOutput, error) {
	p, err := s.store.GetSplitProposal(ctx, in.ProposalID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, httpcore.NotFound("Split proposal not found",
			"That proposal doesn't exist — it may have been confirmed, rejected, or replaced by a re-run of detection.")
	}
	if err != nil {
		return nil, err
	}
	if !p.Ready() {
		return nil, httpcore.NotFound("Split proposal not found",
			"Detection is still running; the proposal will appear here when its cut list is ready.")
	}
	hashes := make([]string, 0, len(p.Segments))
	for _, segment := range p.Segments {
		hashes = append(hashes, segment.ArtworkImageHash)
	}
	byHash := s.imageDTOsByHash(ctx, hashes)
	segments := make([]SplitReviewSegmentDTO, 0, len(p.Segments))
	for _, segment := range p.Segments {
		segments = append(segments, SplitReviewSegmentDTO{
			Segment: segment,
			Artwork: byHash[segment.ArtworkImageHash],
		})
	}
	// The current set only decides whether review offers a recheck, so a failed read must not make
	// the proposal unreviewable. Unknown reads as unchanged: no recheck is offered on a guess.
	current := p.LanguagePreference
	if s.intakeLanguages != nil {
		if read, err := s.intakeLanguages(ctx); err != nil {
			s.log.Warn("split review: could not read the channels' commercial languages", "proposal", p.ID, "err", err)
		} else {
			current = read
		}
	}
	return &getFillerSplitOutput{Body: SplitReviewProposalDTO{
		ID: p.ID, ClipHash: p.ClipHash, CreatedAt: p.CreatedAt, Segments: segments,
		CheckedLanguages:   language.SplitSet(p.LanguagePreference),
		CurrentLanguages:   language.SplitSet(current),
		LanguageExclusions: p.LanguageExclusions,
	}}, nil
}

type confirmFillerSplitInput struct {
	ProposalID string `path:"proposalId"`
	Body       struct {
		// Segments is the operator's REVIEWED cut list: the proposal as returned,
		// possibly edited. It is re-validated server-side (§10 — the review gate's
		// teeth live on the write path, not in the UI).
		// At most 500: the longest compilation reel proposes about fifty.
		Segments []SplitSegment `json:"segments" minItems:"1" maxItems:"500"`
	}
}

// SplitSegment is split.Segment under the schema name the OpenAPI document and its generated
// clients publish for a proposed clip; the schema is named after the Go type, and `Segment` alone
// would say nothing there.
type SplitSegment split.Segment
type confirmFillerSplitOutput struct {
	Body struct {
		// Clips is how many catalog clips the confirm created — the confirmation
		// the UI toasts before the catalog list refreshes.
		Clips int `json:"clips"`
	}
}

// confirmFillerSplit commits the reviewed cut list. Synchronous: stream-copy
// cuts seek rather than decode, so even twenty segments are seconds — the
// minutes belong to detection, which already ran.
func (s *Handlers) confirmFillerSplit(ctx context.Context, in *confirmFillerSplitInput) (*confirmFillerSplitOutput, error) {
	if s.filler == nil {
		return nil, httpcore.NotImplemented("Filler isn't set up",
			"Enable filler in Settings before confirming a split.")
	}
	segments := make([]split.Segment, len(in.Body.Segments))
	for i, segment := range in.Body.Segments {
		segments[i] = split.Segment(segment)
	}
	if err := s.filler.ConfirmSplit(ctx, in.ProposalID, segments); err != nil {
		switch {
		case errors.Is(err, store.ErrNotFound):
			return nil, httpcore.NotFound("Split proposal not found",
				"That proposal doesn't exist — it may have been confirmed already, or replaced by a re-run of detection.")
		case errors.Is(err, filler.ErrSplitParentNotHeld):
			return nil, httpcore.Conflict("This clip is already filed",
				"Only a recording held for split review can be cut. This clip is already in the catalog, so these cuts can't be confirmed — reject the proposal instead.")
		case errors.Is(err, filler.ErrSplitValidation):
			return nil, httpcore.ProblemWithCause(http.StatusUnprocessableEntity, "Invalid cut list",
				"The edited segments are outside the clip, overlap, or too short — fix the cuts and try again.", err)
		default:
			return nil, err
		}
	}
	out := &confirmFillerSplitOutput{}
	out.Body.Clips = len(in.Body.Segments)
	return out, nil
}
