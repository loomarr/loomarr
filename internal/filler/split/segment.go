package split

import (
	"github.com/loomarr/loomarr/internal/filler/clips"
	"github.com/loomarr/loomarr/internal/media/tools"
)

// BoundarySource is which detector(s) put a cut where it is — a bitmask, because the merges are
// naturally unions and because "black AND silence agreed" is the single most useful fact the
// detection pass produces (§10 V34, the confidence ladder).
type BoundarySource uint8

const (
	SrcBlack BoundarySource = 1 << iota
	SrcSilence
	SrcChapter
	SrcTranscript
	// SrcReelEdge is the file's own start or end — declared by the container, not inferred.
	SrcReelEdge
	// SrcTruncated marks an edge an overlap resolution MOVED. The span is real; this particular
	// end of it was chosen by arithmetic rather than observed.
	SrcTruncated
	// SrcOperator is a boundary a human typed. It carries no score — see `boundaryScore`.
	SrcOperator
)

// Segment is one proposed clip inside a compilation (§10 V34). The detector
// authors it; the reviewer edits it; only confirm writes it to the catalog.
type Segment struct {
	Index   int   `json:"index"`
	StartMs int64 `json:"startMs"`
	EndMs   int64 `json:"endMs"`
	// Name is the proposed display name. Its origin says whether it came from the source, exact
	// segment evidence, a deterministic fallback, or the operator. NameEvidence is deliberately
	// descriptive only: it cannot authorize a cut, classification, or admission.
	Name         string `json:"name"`
	NameOrigin   string `json:"nameOrigin,omitempty"`
	NameEvidence string `json:"nameEvidence,omitempty"`
	// Era/Audience/Tags come from the SAME Classify the tag job uses, over the
	// segment's transcript. Era is grounded (year in the text) or zero — an
	// ungrounded guess is carried ONLY as SuggestedEra (§10 era rule).
	Era          int            `json:"era,omitempty"`
	SuggestedEra int            `json:"suggestedEra,omitempty"`
	Audience     clips.Audience `json:"audience,omitempty"`
	// Tags is the grounded taxonomy leaf set (§10 V45a); Category is its DERIVED product-leaf shadow,
	// carried so the review and the confirmed clip both have the cheap read-path value. Both come from
	// the same Classify the tag job uses.
	Tags     []string `json:"tags,omitempty" maxItems:"64"` // at most 64, as on a clip
	Category string   `json:"category,omitempty"`
	// BoundaryConfidence is how much Loomarr trusts WHERE this was cut, 0–100 (§10 V34 ladder).
	// `0` means never scored — every proposal detected before V54 deserialises that way, and the
	// review surface must treat "unscored" as "no opinion", never as "doubtful".
	//
	// It answers "did we cut in the right place?", nothing about what the segment is.
	BoundaryConfidence int `json:"boundaryConfidence,omitempty"`
	// StartEvidence/EndEvidence say WHY, in the operator's words ("black + silence", "silence only").
	//
	// ⚠ **Two fields because a segment has two boundaries**, and they routinely disagree — a cut
	// can start on a chapter marker and end on a lone silence. One field would have to pick a
	// winner, and the number already picks the worse of the two; the evidence is what tells the
	// operator WHICH end to look at, so collapsing it throws away the actionable half.
	StartEvidence string `json:"startEvidence,omitempty"`
	EndEvidence   string `json:"endEvidence,omitempty"`
	// HoldReason is why this segment was kept back when its reel was partly confirmed (§10 V54) —
	// the same vocabulary `AutoSplitReject` uses, so the review says "a segment's era is a guess"
	// rather than leaving the operator to work out why 5 of 52 are still here.
	HoldReason string `json:"holdReason,omitempty"`
	// StartSrc/EndSrc are the bitmask the two evidence fields above are rendered from. Never
	// serialized: the wire carries the rendered tokens and the score, never the bitmask, so adding
	// a source is not a breaking API change.
	StartSrc BoundarySource `json:"-"`
	EndSrc   BoundarySource `json:"-"`
	// RescueConfirmedWhole means the transcript rescue looked at this over-long span and reported
	// exactly ONE advert. It lifts the over-long cap — see `boundaryScore`.
	RescueConfirmedWhole bool `json:"-"`
	// Looked records that the split-time grounder ALREADY examined this segment's frames —
	// whether or not it came back with anything (§10 V54).
	//
	// ⚠ **Inference cannot replace this flag.** `Category != "" || Era > 0` conflates *never
	// looked at* with *looked at and grounded nothing*, and treating those alike is exactly what
	// makes a resumable budget never converge: the ungroundable segments would be retried every
	// pass, forever, and the reel would never reach a verdict.
	//
	// ⚠ It is set even when the frame extraction or the JSON parse FAILED. A transient ffmpeg
	// error therefore retires that segment permanently for this proposal. That is deliberate — it
	// is what bounds the loop — and the remedy is a re-detect, which replaces the proposal. The
	// provider-error path does NOT set it, because a failing backend fails for every segment and
	// should be retried next pass rather than poisoning the reel.
	Looked bool `json:"looked,omitempty"`
	// RoleEvidence is the exact, independently attributable frame judgement for THIS span. It is
	// intentionally not flattened into Category or copied from the parent: the source-structure
	// reducer consumes it as a segment_role observation and decides whether the span is keepable.
	// nil means no valid role evidence was produced, even when Looked is true.
	RoleEvidence *RoleEvidence `json:"-"`
	// DupOf is the path of an existing catalog clip this segment duplicates
	// (dHash, measured 25× separation). Propose records the detection fact; the automated split
	// stage then discards it before classification because it cannot add a new catalog clip.
	DupOf string `json:"dupOf,omitempty"`
	// DedupUnchecked means the comparison against the catalog could not be made for this segment (its
	// fingerprint could not be read, or the catalog could not be listed). It is not a finding: an
	// empty DupOf then reads as "not checked", not as "checked and new", and the review says so. A
	// proposal stored before this field existed reads as checked.
	DedupUnchecked bool `json:"dedupUnchecked,omitempty"`
	// Unsplittable means the segment is over-long AND the rescue could not run or
	// found nothing (no whisper, whisper/LLM failure). The review must say so —
	// the alternative is guessing, which is exactly what the era rule forbids in
	// tag form.
	Unsplittable bool `json:"unsplittable,omitempty"`
	// Transcript is the segment's whisper text, when the rescue ran. Carried on
	// the proposal so the reviewer can SEE why a boundary was proposed; not
	// persisted to the catalog on confirm.
	Transcript string `json:"transcript,omitempty"`
	// Language is the language heard inside this exact proposed span. LanguageChecked keeps a
	// checked-but-unknown answer distinct from a legacy segment that has never been heard.
	Language        string `json:"language,omitempty"`
	LanguageChecked bool   `json:"languageChecked,omitempty"`
	// LanguageReason is a stable machine-readable explanation when a check produced no language.
	// LanguageNote retains optional technical detail for diagnostics; clients must not parse it to
	// decide what happened or expose it as primary copy.
	LanguageReason string `json:"languageReason,omitempty"`
	LanguageNote   string `json:"languageNote,omitempty"`
	// ArtworkImageHash is the shared image-service identity of the representative frame for this
	// exact proposed span. It is persisted beside the proposal but never accepted from the wire:
	// API output resolves it to a complete ImageDTO, while confirmation remains only a cut-list
	// command. ArtworkChecked distinguishes an extraction failure from work that has not run yet.
	ArtworkImageHash string `json:"-"`
	ArtworkChecked   bool   `json:"-"`
}

// DetectionProgress is the private durable checkpoint for coarse boundary detection. It is
// stored with a proposal but never exposed through the review interface: until this becomes nil,
// the proposal is detector work rather than a cut list an operator can judge.
type DetectionProgress struct {
	ScannedThroughMs int64            `json:"scannedThroughMs"`
	Black            []tools.Interval `json:"black,omitempty"`
	Silence          []tools.Interval `json:"silence,omitempty"`
	// ChapterEdges retains every declared edge, including the bounds of a below-floor chapter
	// omitted from CoarseSegments. V67 needs those facts to account for the omitted interval.
	ChapterEdges []int64 `json:"chapterEdges,omitempty"`
	// Discarded retains exact below-floor spans across the durable detection checkpoint. The old
	// in-memory aggregate remains display data; these intervals are structure authority.
	Discarded []tools.Interval `json:"discarded,omitempty"`
	// Chapters says CoarseSegments came from container-authored chapter boundaries. It is private
	// checkpoint provenance, not review data, and lets a resume restore scoring evidence even when
	// a source supplied one untitled chapter (otherwise indistinguishable from a whole-reel
	// boundary fallback after JSON removed the private source bitmask).
	Chapters bool `json:"chapters,omitempty"`
	// CoarseSegments is set once chapter/boundary triage is complete. Persisting it before
	// transcript rescue means a timeout in the next phase never repeats the timeline scan.
	CoarseSegments []Segment `json:"coarseSegments,omitempty"`
}
