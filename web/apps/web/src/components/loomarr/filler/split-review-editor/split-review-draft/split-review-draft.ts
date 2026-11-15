import type { SplitReviewSegmentDTO } from "@loomarr/api/models/splitReviewSegmentDTO";
import type { SplitSegment } from "@loomarr/api/models/splitSegment";
import { formatMmSs, parseMmSs } from "@loomarr/core/format";

// The draft model behind SplitReviewEditor: the editable mm:ss text for each proposed segment,
// how that text resolves back to milliseconds, and the validity rules Confirm depends on.

// The smallest span the BE will cut (internal/filler MinSegmentMs); mirrored here so the
// editor disables Confirm rather than round-tripping a 422 the operator could see coming.
const MIN_SEGMENT_MS = 3000;

// A draft segment is the wire segment plus its editable mm:ss text. Everything else rides
// along untouched so the confirm body is the operator's list verbatim (renumbered). The
// `key` is a STABLE identity for React: keying on position or on the edited times would
// remount the row mid-keystroke and steal focus from the very inputs being edited.
interface DraftSegment extends SplitReviewSegmentDTO {
  key: string;
  startText: string;
  endText: string;
  artwork: SplitReviewSegmentDTO["artwork"];
}

const toDraft = (seg: SplitReviewSegmentDTO): DraftSegment => ({
  ...seg,
  key: `seg-${seg.index}-${seg.startMs}`,
  startText: formatMmSs(seg.startMs),
  endText: formatMmSs(seg.endMs),
  artwork: seg.artwork,
});

// resolveMs turns one edited time field back into milliseconds WITHOUT quantizing an untouched
// boundary to a whole second.
//
// ⚠ The detector proposes sub-second cuts (a black frame lands at 12_345ms, not 12_000ms), but the
// editor renders mm:ss and `formatMmSs` FLOORS. Parsing that text back unconditionally — which is
// what this did — moved EVERY boundary by up to 999ms the moment a proposal was opened, whether or
// not the operator touched it. Merely looking at a reel rewrote its cuts.
//
// The test is "does the text still say what we rendered from this value": if so it is untouched
// and the original precision stands. If the operator typed something else they meant it, and
// mm:ss is the precision they were given to express it in.
const resolveMs = (text: string, originalMs: number): number => {
  if (text === formatMmSs(originalMs)) return originalMs;
  // ⚠ Falls back to the ORIGINAL, not to 0. `?? 0` would commit a half-typed or invalid field
  // ("1:") as a segment starting at zero — silently, since `isValid` blocks the Confirm button
  // only while the row is on screen. The filmstrip twenty lines below falls back to `d.startMs`
  // too, so the two agree about the same input.
  return parseMmSs(text) ?? originalMs;
};

// toWire renumbers: drops and merges change the ORDER, and the body the operator commits
// is indexed by the draft they see, not by the detector's original numbering.
const toWire = (draft: DraftSegment[]): SplitSegment[] =>
  draft.map((d, i) => {
    const { key: _k, startText: _s, endText: _e, artwork: _a, ...seg } = d;
    return {
      ...seg,
      index: i,
      startMs: resolveMs(d.startText, d.startMs),
      endMs: resolveMs(d.endText, d.endMs),
    };
  });

const spanMs = (d: DraftSegment): number | undefined => {
  // Validation reads the same resolved values the wire body will carry, so a segment cannot pass
  // the min-duration check at one precision and be committed at another.
  if (parseMmSs(d.startText) === undefined || parseMmSs(d.endText) === undefined) return undefined;
  return resolveMs(d.endText, d.endMs) - resolveMs(d.startText, d.startMs);
};

const isValid = (d: DraftSegment): boolean => {
  const span = spanMs(d);
  return span !== undefined && span >= MIN_SEGMENT_MS && d.name.trim() !== "";
};

const AUDIENCE_LABEL: Record<string, string> = {
  kids: "Kids",
  family: "Family",
  general: "General",
  late_night: "Late night",
};

const languageNeedsRecheck = {
  language: "",
  languageChecked: false,
  languageReason: "",
  languageNote: "Language will be checked again after this edit.",
} as const;

const nameNowOwnedByOperator = {
  nameOrigin: "operator-edited",
  nameEvidence: undefined,
} as const;

const nameDetail = (segment: SplitReviewSegmentDTO): string | undefined => {
  switch (segment.nameOrigin) {
    case "model-proposed":
      return segment.nameEvidence
        ? `Name suggested from this clip: “${segment.nameEvidence}”`
        : "Name suggested from this clip.";
    case "source-authored":
      return "Name provided by the source.";
    case "fallback":
      return "Temporary name based on the recording.";
    case "operator-edited":
      return "Name edited during review.";
    default:
      return undefined;
  }
};

const unresolvedLanguageMessage = (segment: SplitReviewSegmentDTO): string | undefined => {
  switch (segment.languageReason) {
    case "unavailable":
      return "Language wasn’t checked because speech recognition isn’t set up. This clip is still included.";
    case "failed":
      return "Language couldn’t be checked because speech recognition had a problem. This clip is still included.";
    case "inconclusive":
      return "Loomarr heard speech but couldn’t confidently identify the language. This clip is still included.";
    case "paused":
      return "Language checking is paused by the processing limit. This clip is still included.";
    default:
      if (segment.languageNote === languageNeedsRecheck.languageNote)
        return languageNeedsRecheck.languageNote;
      if (segment.languageNote) return "Language couldn’t be confirmed. This clip is still included.";
      return undefined;
  }
};

export type { DraftSegment };
export {
  AUDIENCE_LABEL,
  isValid,
  languageNeedsRecheck,
  nameDetail,
  nameNowOwnedByOperator,
  resolveMs,
  spanMs,
  toDraft,
  toWire,
  unresolvedLanguageMessage,
};
