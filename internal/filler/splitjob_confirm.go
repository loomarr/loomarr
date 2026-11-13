package filler

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"time"

	"github.com/loomarr/loomarr/internal/filler/asset"
	"github.com/loomarr/loomarr/internal/filler/cliplibrary"
	"github.com/loomarr/loomarr/internal/filler/pipelinestate"
	"github.com/loomarr/loomarr/internal/filler/research"
	"github.com/loomarr/loomarr/internal/filler/split"
	"github.com/loomarr/loomarr/internal/media/tools"
	"github.com/loomarr/loomarr/internal/platform/storagegovernor"
	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
)

// Confirm writes the operator's reviewed cut list to the catalog (§10): each kept segment is cut
// with stream copy into the clip folder and becomes a child row. The compilation remains as a
// non-airable composite for lineage and future re-splitting; the proposal is consumed. On a
// re-split, final confirmation tombstones the superseded child generation without deleting bytes.
//
// The segments arrive operator-edited and are re-validated: inside the clip,
// start<end, non-overlapping. Anything else is an error, not a best effort —
// this is the write path, and the review gate is the whole point of the phase.
//
// It returns the HASHES of the clips it created, in cut order (§10 V51b). They are what the
// ingest pipeline enrols, so each segment runs the full ladder for itself; a caller that does not
// need them can ignore the slice.
// Confirm cuts `segments` out of the compilation and files them, deleting the proposal.
//
// The MANUAL path: an operator has finished with this reel and their list is the whole answer.
func (sp *Splitter) Confirm(ctx context.Context, proposalID string, segments []split.Segment) ([]string, error) {
	return sp.confirm(ctx, proposalID, segments, nil, storagegovernor.ConfirmedManual)
}

// ConfirmSome cuts `segments` and leaves `hold` behind in a shrunken proposal (§10 V54).
//
// The AUTOMATIC path: the gate confirmed the cuts it was confident about and kept the rest for a
// human. ⚠ `hold` is what SURVIVES, stated by the caller rather than diffed from the store — see
// the reasoning at the write below.
func (sp *Splitter) ConfirmSome(ctx context.Context, proposalID string, segments, hold []split.Segment) ([]string, error) {
	return sp.confirm(ctx, proposalID, segments, hold, storagegovernor.Automatic)
}

// splitProposalClaimLease is deliberately longer than the bounded local split operation. A crash
// is recoverable after the deadline; a live owner renews immediately before making sidecars or
// media visible, so an expired predecessor is fenced before it can publish.
const splitProposalClaimLease = 30 * time.Minute

// confirm runs the three steps of a confirmation in order: claim the proposal so one confirmer owns
// it, cut the reviewed segments into hidden staging, and publish them (record the children and
// consume or shrink the proposal). Staging and the claim outlive the cut step on purpose: the cut
// bytes are only moved into place by the publish step, and the claim fences a predecessor until
// then.
func (sp *Splitter) confirm(ctx context.Context, proposalID string, segments, hold []split.Segment, storageMode storagegovernor.Mode) ([]string, error) {
	claim, err := sp.claimSplitProposal(ctx, proposalID)
	if err != nil {
		return nil, err
	}
	defer claim.release()
	plan, err := sp.planSplitConfirmation(ctx, proposalID, claim, segments, hold)
	if err != nil {
		return nil, err
	}
	staging := &splitStaging{}
	defer staging.close()
	cuts, err := sp.cutConfirmedSegments(ctx, claim, plan, storageMode, staging)
	if err != nil {
		return nil, err
	}
	return sp.publishConfirmedSegments(ctx, proposalID, claim, plan, cuts)
}

// splitClaim is a confirmer's lease on one proposal: the token that fences it, the proposal as it
// was claimed, and the release that hands the lease back.
type splitClaim struct {
	token    string
	proposal SplitProposal
	release  func()
}

func (sp *Splitter) claimSplitProposal(ctx context.Context, proposalID string) (*splitClaim, error) {
	if sp.newID == nil {
		return nil, errors.New("split confirm: claim token generator is required")
	}
	claimToken := sp.newID()
	if claimToken == "" {
		return nil, errors.New("split confirm: claim token is empty")
	}
	claimedAt := sp.now().UTC()
	p, err := sp.store.AcquireSplitProposalClaim(ctx, proposalID, claimToken, claimedAt, claimedAt.Add(splitProposalClaimLease))
	if err != nil {
		return nil, err
	}
	release := func() {
		releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = sp.store.ReleaseSplitProposalClaim(releaseCtx, proposalID, claimToken)
	}
	return &splitClaim{token: claimToken, proposal: p, release: release}, nil
}

// splitConfirmPlan is a confirmation after validation: the compilation, the source bytes it is cut
// from, and the reviewed segments with the server-owned evidence rebound onto them.
type splitConfirmPlan struct {
	clip     clipstore.Clip
	source   split.SourceAsset
	src      string
	segments []split.Segment
	hold     []split.Segment
}

func (sp *Splitter) planSplitConfirmation(ctx context.Context, proposalID string, claim *splitClaim, segments, hold []split.Segment) (*splitConfirmPlan, error) {
	p := claim.proposal
	if !p.Ready() {
		return nil, fmt.Errorf("%w: proposal %s is still detecting boundaries", ErrSplitValidation, proposalID)
	}
	// Language is detector evidence, not an operator-editable field. The confirm body carries the
	// whole segment shape for one generated contract, but only an exact persisted interval may
	// reuse its answer. A hand-edited or merged interval is a different audio span and deliberately
	// reaches the ordinary post-confirm language rung with no answer.
	segments = bindSplitLanguageEvidence(segments, p.Segments)
	hold = bindSplitLanguageEvidence(hold, p.Segments)
	// Naming provenance is server-authored too. An unchanged exact span recovers the persisted
	// source/model evidence; any renamed, resized, or merged clip keeps the operator's display name
	// but cannot claim the machine's evidence for different content.
	segments = bindSplitNameEvidence(segments, p.Segments)
	hold = bindSplitNameEvidence(hold, p.Segments)
	// Artwork is also server-authored evidence. Preserve it only when an automatically held span
	// still names the exact interval that produced the frame; an edited interval must return to the
	// bounded artwork preparation pass rather than displaying a still from different content.
	hold = bindSplitArtworkEvidence(hold, p.Segments)
	clip, found, err := sp.store.GetClip(ctx, p.ClipHash)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("compilation %s no longer in the catalog", p.ClipHash)
	}
	if len(segments) == 0 {
		return nil, fmt.Errorf("%w: zero segments — reject the proposal instead of gutting the compilation", ErrSplitValidation)
	}
	// V66 freezes the exact evidence location and full digest in the proposal. Pre-V66 proposals
	// resolve once through the catalog row and are upgraded to the same bound source shape.
	source, src, err := resolveSplitSource(ctx, sp.dropDir, clip, p.Source)
	if err != nil {
		return nil, fmt.Errorf("split confirm: %w", err)
	}
	if err := validateConfirmedSegments(segments, source.DurationMs, sp.floor()); err != nil {
		return nil, err
	}
	return &splitConfirmPlan{clip: clip, source: source, src: src, segments: segments, hold: hold}, nil
}

// splitStaging collects what the cut step must keep alive until the publish step is done (the
// storage lease, the staging directory, the write monitor) and releases it in reverse order.
type splitStaging struct{ cleanups []func() }

func (s *splitStaging) after(cleanup func()) { s.cleanups = append(s.cleanups, cleanup) }

func (s *splitStaging) close() {
	for i := len(s.cleanups) - 1; i >= 0; i-- {
		s.cleanups[i]()
	}
}

// splitCuts is what the cut step hands the publish step: the prepared cuts and the snapshot of the
// compilation they were cut from.
type splitCuts struct {
	publication splitPublication
	snapshot    string
}

// cutConfirmedSegments reserves storage, snapshots the compilation and cuts every reviewed segment
// into the hidden staging directory. It writes nothing the catalog can see.
func (sp *Splitter) cutConfirmedSegments(ctx context.Context, claim *splitClaim, plan *splitConfirmPlan, storageMode storagegovernor.Mode, staging *splitStaging) (*splitCuts, error) {
	p, clip, source, src, segments := claim.proposal, plan.clip, plan.source, plan.src, plan.segments
	claimToken := claim.token
	var storageLease *storagegovernor.Lease
	if sp.storage != nil {
		budget, ok := storagegovernor.EstimateMedia(storagegovernor.MediaEstimate{
			DeclaredBytes: source.Bytes, DurationMS: source.DurationMs,
		})
		if !ok {
			return nil, storagegovernor.Pause("estimate split storage", storagegovernor.Decision{
				Snapshot: storagegovernor.Snapshot{Domain: storagegovernor.DomainFiller, Reason: storagegovernor.ReasonEstimateUnknown},
			})
		}
		var decision storagegovernor.Decision
		storageLease, decision = sp.storage.Reserve(ctx, storagegovernor.Request{
			Path: src, Domain: storagegovernor.DomainFiller,
			EstimatedBytes: budget.ReservationBytes, Mode: storageMode,
		})
		if storageLease == nil {
			return nil, storagegovernor.Pause("reserve split storage", decision)
		}
		staging.after(storageLease.Release)
		if decision = storageLease.Revalidate(ctx, 0); !decision.Allowed {
			return nil, storagegovernor.Pause("revalidate split storage", decision)
		}
	}
	ext := filepath.Ext(source.Path)
	parentPlayable := filepath.Join(sp.dropDir, filepath.FromSlash(clip.Path))
	// A damaged parent sidecar holds the confirm: absence is ordinary (zero tags), but cutting from
	// unreadable provenance would publish children with no source or acquisition identity.
	parentTags, parentSidecar := asset.ReadSidecarTagsState(parentPlayable)
	if parentSidecar == asset.SidecarInvalid {
		return nil, fmt.Errorf("split confirm: parent sidecar for %s is unreadable; held until it is repaired", clip.Path)
	}

	// Segments are cut inside a hidden directory on the filler filesystem. Same-filesystem staging
	// makes final publication atomic, while ScanDir's dot-directory rule keeps partial bytes out of
	// the catalog. The final-bound lineage sidecar is linked first; only then does the media name
	// appear, so scanning cannot observe an unbound child.
	stageRoot := filepath.Join(sp.dropDir, ".split")
	if err := os.MkdirAll(stageRoot, 0o700); err != nil {
		return nil, fmt.Errorf("split confirm: create staging root: %w", err)
	}
	tmpDir, err := os.MkdirTemp(stageRoot, "confirm-")
	if err != nil {
		return nil, fmt.Errorf("split confirm: temp dir: %w", err)
	}
	staging.after(func() { _ = os.RemoveAll(tmpDir) })
	splitCtx := ctx
	finishStorage := func() error { return nil }
	if storageLease != nil {
		splitCtx, finishStorage = storagegovernor.MonitorPath(ctx, storageLease, tmpDir, 0)
	}
	staging.after(func() { _ = finishStorage() })
	sourceSnapshot, err := snapshotSplitComposite(splitCtx, src, tmpDir, ext, source.ClipHash)
	if err != nil {
		if storageErr := finishStorage(); storageErr != nil {
			return nil, storageErr
		}
		return nil, err
	}

	// Cut everything FIRST: a failure mid-confirm leaves the compilation intact
	// (the proposal is only consumed once every segment exists on disk and in
	// the catalog), so the operator can fix and retry rather than losing cuts.
	publication := splitPublication{token: claimToken}
	for i, seg := range segments {
		childKind := clip.Role
		structureAuthority, hasStructureAuthority := structureDecisionAuthorityForInterval(p, seg)
		if hasStructureAuthority {
			var mapped bool
			childKind, mapped = catalogRoleForStructureRole(structureAuthority.role)
			if !mapped {
				return nil, fmt.Errorf("split confirm: certified segment %d has no catalog kind", i)
			}
		}
		tmp := filepath.Join(tmpDir, fmt.Sprintf("seg-%03d%s", i, ext))
		if err := sp.tools.Cut(splitCtx, sourceSnapshot, seg.StartMs, seg.EndMs, tmp); err != nil {
			if storageErr := finishStorage(); storageErr != nil {
				return nil, storageErr
			}
			return nil, err
		}
		id, err := asset.ClipID(tmp)
		if err != nil {
			return nil, fmt.Errorf("split confirm: hash segment %d: %w", i, err)
		}
		dst, err := cliplibrary.ClipPath(sp.dropDir, id, ext)
		if err != nil {
			return nil, err
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return nil, fmt.Errorf("split confirm: segment %d: %w", i, err)
		}
		childTags := asset.SidecarTags{
			SourceID:              parentTags.SourceID,
			AcquisitionID:         parentTags.AcquisitionID,
			OriginalName:          seg.Name + ext,
			Kind:                  string(childKind),
			SplitPublicationToken: claimToken,
			ConditioningLineage: &asset.ConditioningLineage{
				ChildHash:               id,
				ParentHash:              clip.Hash,
				ParentAssetRole:         string(source.Role),
				ParentAssetSHA256:       source.SHA256,
				StructureDecisionSHA256: structureAuthority.sha256,
				StructureRole:           structureAuthority.role,
				IntendedStartMs:         seg.StartMs,
				IntendedEndMs:           seg.EndMs,
			},
		}
		// The segment's name and where it came from travel beside the bytes, so a catalog rebuild
		// still knows a chapter title or an operator's edit from the filename fallback (#1988).
		// originalName keeps the name too: era grounding reads it as the clip's text.
		if nameSource := segmentNameSource(seg.NameOrigin); nameSource.Known() {
			childTags.NameProvenance = string(nameSource)
			if nameSource.ShownToViewers() {
				childTags.DisplayName = seg.Name
			}
		}
		if err := asset.WriteSidecarTags(tmp, childTags, false); err != nil {
			return nil, fmt.Errorf("split confirm: stage segment provenance: %w", err)
		}
		// ⚠ An existing destination is a byte-identical cut, not a collision: the path IS the
		// hash. The old code guarded this with a `-2`, `-3` suffix loop over operator-shaped
		// names, which content addressing makes both unnecessary and wrong. Reuse is safe only
		// when the existing sidecar names this exact reviewed child/parent/interval; the catalog
		// has one lineage slot and must not silently replace it for identical bytes from elsewhere.
		if _, statErr := os.Stat(dst); errors.Is(statErr, os.ErrNotExist) {
		} else if statErr != nil {
			return nil, fmt.Errorf("split confirm: inspect segment %d: %w", i, statErr)
		} else {
			equal, compareErr := asset.ExactFileBytesEqual(splitCtx, tmp, dst, tools.ConditioningMaxSnapshotBytes)
			if compareErr != nil || !equal {
				return nil, fmt.Errorf("split confirm: existing segment %d bytes do not match identity", i)
			}
			existingTags, ok := asset.ReadSidecarTags(dst)
			if !ok || !reflect.DeepEqual(existingTags.ConditioningLineage, childTags.ConditioningLineage) {
				return nil, fmt.Errorf("split confirm: existing segment %d is not bound to this reviewed cut", i)
			}
			if err := asset.WriteSidecarTags(dst, childTags, false); err != nil {
				return nil, fmt.Errorf("split confirm: fence existing segment %d: %w", i, err)
			}
			publication.cuts = append(publication.cuts, preparedSplitCut{segment: seg, kind: childKind, hash: id, path: cliplibrary.ClipRelPath(id, ext), staged: tmp, final: dst, existing: true})
			continue
		}
		publication.cuts = append(publication.cuts, preparedSplitCut{segment: seg, kind: childKind, hash: id, path: cliplibrary.ClipRelPath(id, ext), staged: tmp, final: dst})
	}
	if err := finishStorage(); err != nil {
		return nil, err
	}
	return &splitCuts{publication: publication, snapshot: sourceSnapshot}, nil
}

// publishConfirmedSegments moves the cut segments into place and records them: it fences the
// source, publishes the staged files, files each child in the catalog (held until the generation
// commits), and then either consumes the proposal or shrinks it to the segments still held for a
// person. It returns the hashes of the clips it created.
func (sp *Splitter) publishConfirmedSegments(ctx context.Context, proposalID string, claim *splitClaim, plan *splitConfirmPlan, cuts *splitCuts) ([]string, error) {
	p, clip, src, hold, claimToken := claim.proposal, plan.clip, plan.src, plan.hold, claim.token
	publication := cuts.publication
	if err := validateSplitCompositeOwnership(ctx, src, cuts.snapshot); err != nil {
		return nil, fmt.Errorf("split confirm: composite source changed while cuts were prepared: %w", err)
	}
	if err := sp.store.RenewSplitProposalClaim(ctx, proposalID, claimToken, sp.now().UTC().Add(splitProposalClaimLease)); err != nil {
		return nil, err
	}
	if err := publication.prepare(ctx); err != nil {
		publication.rollback()
		return nil, err
	}
	defer publication.rollback()
	now := sp.now().UTC()
	// ⚠ The hashes are RETURNED, not merely written. The ingest pipeline enrols each one at
	// `probe` so a fresh segment runs the whole ladder for itself (§10 V51b) — before this, a cut
	// advert had to wait for whichever of six catalog sweeps happened to reach it next.
	spawned := make([]string, 0, len(publication.cuts))
	for _, c := range publication.cuts {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		spawned = append(spawned, c.hash)
		nc := clipstore.Clip{UpdatedAt: now}
		// ⚠ **The identity must be set.** `UpsertClip` is `ON CONFLICT(hash) DO UPDATE`, so a
		// segment inserted with `hash=''` overwrites the last one — a 41-segment reel would become
		// ONE catalog row. Invisible to the splitter's own tests because their store keys its map
		// on `Path`; the store conformance suite pins this class.
		nc.Hash = c.hash
		nc.Path = c.path
		nc.Name, nc.NameSource = c.segment.Name, segmentNameSource(c.segment.NameOrigin)
		nc.Role = c.kind
		nc.DurationMs = c.segment.EndMs - c.segment.StartMs
		nc.Era = c.segment.Era
		nc.Audience = c.segment.Audience
		// ⚠ Lineage: each segment points back at the composite it was cut from (§10 V45). This is what
		// V45 keeps that V34 discarded — provenance ("which break did this advert air in?"), a
		// re-split when detection improves, and broadcast-context inheritance. `clip.Hash` is the
		// composite's identity (the parent is kept, below, not deleted).
		nc.ParentHash = clip.Hash
		// A reviewed child is held and tombstoned before its media name is published. The pipeline
		// may process it, while ReplaceSplitChildren remains the atomic generation selector.
		nc.Held = true
		nc.RemovedAt = now
		// Persist the transcript the rescue step already produced (§10 V44). Pre-V44 this was
		// computed to find ad boundaries and then thrown away; it is the richest metadata signal a
		// split segment has — a segment with no source description still SAYS its brand — so it
		// carries onto the clip row instead of being re-derived by the transcribe job later.
		nc.Transcript = c.segment.Transcript
		// Reuse a completed per-segment language answer. The child starts again at probe, and the
		// ordinary language rung treats a non-empty code (including `none`) as already heard.
		// Checked-but-unknown stays empty so that defense rung may try again on the rendered child.
		if c.segment.LanguageChecked {
			nc.Language = c.segment.Language
		}
		// Provenance inherits from the compilation: same source, same declared
		// licence (the segments ARE the source's content), same resolution.
		nc.Source = clip.Source
		nc.License = clip.License
		nc.Quality = clip.Quality
		if err := sp.store.UpsertClip(ctx, nc); err != nil {
			return nil, err
		}
		// A segment's guessed era is carried onto the child as an uncited Context suggestion, not
		// a clip fact; the store skips it when the child already has a verified era.
		if c.segment.Era == 0 {
			if err := sp.store.RecordFillerEraObservation(ctx, c.hash, c.segment.SuggestedEra,
				research.ObservedBySplit, now); err != nil {
				return nil, err
			}
		}
		// Persist grounded proposal tags now that the cut has a stable content hash. Union with an
		// identical existing clip so deduplication never lets AI erase operator-authored knowledge.
		segmentTags := append([]string(nil), c.segment.Tags...)
		if len(segmentTags) == 0 && c.segment.Category != "" {
			// Upgrade compatibility for proposals stored before the wire carried the tag set.
			segmentTags = append(segmentTags, c.segment.Category)
		}
		if len(segmentTags) > 0 {
			existing, err := sp.store.GetClipTags(ctx, c.hash, true)
			if err != nil {
				return nil, err
			}
			if err := sp.store.SetClipTags(ctx, c.hash, unionLeaves(existing, segmentTags)); err != nil {
				return nil, err
			}
		}
		if err := sp.store.UpsertClipPipeline(ctx, pipelinestate.ClipPipeline{
			ClipHash: c.hash, Stage: pipelinestate.StageProbe, Status: pipelinestate.StatusQueued,
			Disposition: pipelinestate.DispositionReview, EnrolledAt: now, UpdatedAt: now,
			PreparationAttempt: 1, PreparationStartedAt: now,
			PreparationStartReason: pipelinestate.PreparationStartedByEnrollment, PreparationProgress: 0,
			StageQueuedAt: now,
		}); err != nil {
			return nil, err
		}
	}
	// ⚠ V45 KEEPS the parent, marking it a composite — it does NOT delete the row or the file (the
	// reversal of V34). The composite is not airable (pod assembly excludes `is_composite`), so it
	// harms nothing by staying, and keeping it is what makes the segments' `parent_hash` resolve, a
	// re-split possible, and the recorded break's provenance answerable. Keyed by the composite's
	// HASH (its identity), not its path.
	//
	// The file stays on disk too: a re-scan finds it, but UpsertClip omits `is_composite` from its
	// DO UPDATE list, so the re-synced row keeps its composite mark rather than reverting to airable.
	// ⚠ **`hold` decides whether the proposal survives, and the CALLER supplies it — it is never
	// inferred from `segments`** (§10 V54).
	//
	// The two callers mean different things by the same cut list. An operator clicking Confirm is
	// saying "this reel is finished, cut exactly this", and their list is EDITED — spans retyped,
	// segments merged — so nothing in it need match what was stored. The gate is saying "cut these,
	// keep the rest for a human". Diffing the stored segments against the confirmed ones would read
	// the operator's edits as leftovers and resurrect a reel they had just finished.
	currentGeneration := appendUniqueStrings(p.Spawned, spawned...)
	if len(hold) == 0 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		// The review disposition held the parent out of every catalog read. Once the proposal is
		// fully consumed, file that parent so it can appear as the lineage container around the
		// generated clips. It remains non-airable because SetClipComposite above is the catalog's
		// independent airability chokepoint. Keep partial proposals held: their replacement
		// generation is not committed yet and Incoming still owns the decision.
		if err := sp.store.RenewSplitProposalClaim(ctx, proposalID, claimToken, sp.now().UTC().Add(splitProposalClaimLease)); err != nil {
			return nil, err
		}
		if err := publication.publish(ctx); err != nil {
			return nil, err
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if _, err := sp.store.CompleteSplitConfirmation(ctx, SplitCompletion{
			ProposalID:     proposalID,
			ClaimToken:     claimToken,
			ParentHash:     clip.Hash,
			ChildHashes:    currentGeneration,
			ActivateHashes: spawned,
			At:             now,
		}); err != nil {
			return nil, err
		}
		publication.retain()
		return spawned, nil
	}
	if err := sp.store.SetClipComposite(ctx, clip.Hash, true, now); err != nil {
		return nil, err
	}
	// Renumber so the review's "#N" runs 1..n rather than showing gaps where the confirmed cuts
	// were. Names are untouched: a reel confirmed over two sittings must not rename its
	// remaining clips underneath the operator.
	remaining := append([]split.Segment(nil), hold...)
	for i := range remaining {
		remaining[i].Index = i
	}
	p.Segments = remaining
	p.ArtworkPrepared = splitArtworkPrepared(remaining)
	p.Spawned = currentGeneration
	if err := sp.store.RenewSplitProposalClaim(ctx, proposalID, claimToken, sp.now().UTC().Add(splitProposalClaimLease)); err != nil {
		return nil, err
	}
	if err := publication.publish(ctx); err != nil {
		return nil, err
	}
	if err := sp.store.CompletePartialSplitConfirmation(ctx, SplitPartialCompletion{
		Proposal: p, ClaimToken: claimToken, ActivateHashes: spawned, At: now,
	}); err != nil {
		return nil, err
	}
	publication.retain()
	return spawned, nil
}

func bindSplitLanguageEvidence(submitted, persisted []split.Segment) []split.Segment {
	type span struct{ start, end int64 }
	bySpan := make(map[span]split.Segment, len(persisted))
	for _, segment := range persisted {
		bySpan[span{segment.StartMs, segment.EndMs}] = segment
	}
	out := append([]split.Segment(nil), submitted...)
	for i := range out {
		out[i].Language = ""
		out[i].LanguageChecked = false
		out[i].LanguageReason = ""
		out[i].LanguageNote = ""
		original, ok := bySpan[span{out[i].StartMs, out[i].EndMs}]
		if !ok {
			continue
		}
		out[i].Language = original.Language
		out[i].LanguageChecked = original.LanguageChecked
		out[i].LanguageReason = original.LanguageReason
		out[i].LanguageNote = original.LanguageNote
	}
	return out
}

func bindSplitNameEvidence(submitted, persisted []split.Segment) []split.Segment {
	type span struct{ start, end int64 }
	bySpan := make(map[span]split.Segment, len(persisted))
	for _, segment := range persisted {
		bySpan[span{segment.StartMs, segment.EndMs}] = segment
	}
	out := append([]split.Segment(nil), submitted...)
	for i := range out {
		original, exactSpan := bySpan[span{out[i].StartMs, out[i].EndMs}]
		if exactSpan && out[i].Name == original.Name {
			out[i].NameOrigin = original.NameOrigin
			out[i].NameEvidence = original.NameEvidence
			continue
		}
		out[i].NameOrigin = SplitNameOperatorEdited
		out[i].NameEvidence = ""
	}
	return out
}

func bindSplitArtworkEvidence(submitted, persisted []split.Segment) []split.Segment {
	type span struct{ start, end int64 }
	bySpan := make(map[span]split.Segment, len(persisted))
	for _, segment := range persisted {
		bySpan[span{segment.StartMs, segment.EndMs}] = segment
	}
	out := append([]split.Segment(nil), submitted...)
	for i := range out {
		out[i].ArtworkImageHash = ""
		out[i].ArtworkChecked = false
		original, ok := bySpan[span{out[i].StartMs, out[i].EndMs}]
		if !ok {
			continue
		}
		out[i].ArtworkImageHash = original.ArtworkImageHash
		out[i].ArtworkChecked = original.ArtworkChecked
	}
	return out
}

func splitArtworkPrepared(segments []split.Segment) bool {
	for _, segment := range segments {
		if !segment.ArtworkChecked {
			return false
		}
	}
	return true
}

func appendUniqueStrings(existing []string, values ...string) []string {
	out := append([]string(nil), existing...)
	seen := make(map[string]struct{}, len(out)+len(values))
	for _, value := range out {
		seen[value] = struct{}{}
	}
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

// validateConfirmedSegments enforces the invariants the write path needs:
// inside the clip, start<end, ordered and non-overlapping.
//
// ⚠ The floor here is the COMPOSED one, so the manual path refuses what the scan boundary would
// refuse. A bare `MinSegmentMs` (3s) would let a hand-drawn 8s cut be written, spawned, and then
// rejected `too_short` by the probe rung — a silent downstream loss the operator would never
// connect to their edit. Refusing it upfront is arguable; losing it quietly is not.
func validateConfirmedSegments(segs []split.Segment, durationMs int64, floor segmentFloor) error {
	sorted := append([]split.Segment(nil), segs...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].StartMs < sorted[j].StartMs })
	for i, s := range sorted {
		if s.StartMs < 0 || s.EndMs > durationMs || !floor.admits(s.StartMs, s.EndMs) {
			return fmt.Errorf("%w: segment %d [%d,%d) outside the clip or under %dms (filler.min_duration)", ErrSplitValidation, i, s.StartMs, s.EndMs, floor.ms())
		}
		if i > 0 && s.StartMs < sorted[i-1].EndMs {
			return fmt.Errorf("%w: segments %d and %d overlap — two clips cannot share seconds", ErrSplitValidation, i-1, i)
		}
	}
	return nil
}

// ⚠ `uniqueClipPath` and `sanitizeClipName` were DELETED in V51a, and their absence is the point.
// They existed to turn a segment's display name into a collision-free filename
// ("McDonald's — 1987!" → `mcdonalds-1987.mp4`, then `-2`, `-3` on collision). Under content
// addressing a segment's location IS its hash, so there is no name to sanitise and no collision
// to break: two cuts that produce the same bytes are the same clip and belong in one row. The
// segment's name still travels — on the catalog row, where it is read by humans rather than by
// the filesystem.

// sliceTranscript keeps the utterances overlapping [startMs,endMs) — the text a
// rescued sub-segment is classified from.
func sliceTranscript(transcript []tools.TranscriptSegment, startMs, endMs int64) []tools.TranscriptSegment {
	var out []tools.TranscriptSegment
	for _, t := range transcript {
		if t.EndMs > startMs && t.StartMs < endMs {
			out = append(out, t)
		}
	}
	return out
}
