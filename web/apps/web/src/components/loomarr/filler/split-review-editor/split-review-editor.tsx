import type { SplitReviewSegmentDTO } from "@loomarr/api/models/splitReviewSegmentDTO";
import type { SplitSegment } from "@loomarr/api/models/splitSegment";
import { formatClipDuration, formatMmSs, parseMmSs, pluralize } from "@loomarr/core/format";
import { Merge, Pencil, Trash2 } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { languageName } from "@/lib/languages";
import { cn } from "@/lib/utils";
import { SegmentFilmstrip } from "../segment-filmstrip";
import { SegmentPreview } from "../segment-preview";
import type { SplitReviewEditorProps } from "./split-review-editor.type";

// SplitReviewEditor — the §10 V34 review gate. Detection quality is a property of the
// SOURCE (measured 69–100%), so nothing enters the catalog unconfirmed: this is where the
// operator reads each proposed segment, fixes names and cut points, drops the junk, merges
// what detection over-split, and answers the era questions the grounding validator asked.
//
// The draft holds start/end as TEXT (mm:ss) so an in-progress edit can't corrupt a number;
// parsing happens at confirm time, and an unparseable or inverted span disables Confirm —
// a wrong cut is exactly what this gate exists to catch.

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
  // ⚠ Falls back to the ORIGINAL, not to 0. This used to be `?? 0`, so a half-typed or invalid
  // field ("1:") committed that segment as starting at zero — silently, since `isValid` blocks
  // the Confirm button only while the row is on screen. The filmstrip twenty lines below already
  // fell back to `d.startMs`; the two disagreed about the same input.
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

const SplitReviewEditor = ({
  proposal,
  minClipDurationMs,
  confirming,
  onConfirm,
  onBack,
  className,
}: SplitReviewEditorProps) => {
  const [draft, setDraft] = useState<DraftSegment[]>(() => (proposal.segments ?? []).map(toDraft));
  // Which block the strip has focused. Local, not URL state: it is a pointer at a row on screen,
  // and a shared link carrying it would deep-link someone to a segment index that a merge or a
  // drop has since renumbered.
  const [focusedKey, setFocusedKey] = useState<string>();
  // ⚠ At most ONE preview open, held here rather than per row — see the prop's comment below.
  const [previewKey, setPreviewKey] = useState<string>();

  const setSegment = (i: number, patch: Partial<DraftSegment>) =>
    setDraft((prev) => prev.map((d, j) => (j === i ? { ...d, ...patch } : d)));

  const drop = (i: number) => setDraft((prev) => prev.filter((_, j) => j !== i));

  // Merge with next CONCATENATES THE SPANS (end becomes the next segment's end). Tags keep
  // the first segment's, inheriting from the next only what the first lacks; transcripts
  // join so the reviewer keeps the evidence for the new, longer span.
  // ⚠ `category` (kept here as first-wins, matching every other single-value field) is a
  // DERIVED shadow (§10 V45a) — this review gate never writes it directly, so first-wins is
  // just which half's shadow happens to display until the confirmed segment is re-tagged.
  const mergeWithNext = (i: number) =>
    setDraft((prev) => {
      if (i + 1 >= prev.length) return prev;
      const a = prev[i];
      const b = prev[i + 1];
      if (!a || !b) return prev;
      const merged: DraftSegment = {
        ...a,
        key: `${a.key}+${b.key}`,
        endText: b.endText,
        era: a.era || b.era || undefined,
        suggestedEra: a.suggestedEra || b.suggestedEra || undefined,
        audience: a.audience || b.audience || undefined,
        category: a.category || b.category || undefined,
        dupOf: a.dupOf || b.dupOf || undefined,
        unsplittable: a.unsplittable || b.unsplittable || undefined,
        transcript: [a.transcript, b.transcript].filter(Boolean).join("\n") || undefined,
        artwork: undefined,
        ...languageNeedsRecheck,
        ...nameNowOwnedByOperator,
      };
      return [...prev.slice(0, i), merged, ...prev.slice(i + 2)];
    });

  const confirmable = draft.length > 0 && draft.every(isValid);

  // ⚠ The strip reads the DRAFT's edited timecodes, not the proposal's original spans, so a
  // merge widens a block and a retyped cut point moves one AS IT HAPPENS. Reading the server's
  // copy would leave the picture describing a split the operator already changed.
  //
  // ⚠ Parsed with a FALLBACK to the committed ms. `startText`/`endText` are free text the
  // operator is mid-way through typing — "1:" is not a time yet — and letting an in-progress
  // keystroke collapse a block to zero would make the strip flicker on every character.
  const stripSegments = draft.map((d) => ({
    key: d.key,
    // Same resolution the wire body uses, so the picture and the commit cannot disagree about
    // where a block starts — an untouched boundary keeps its sub-second position in both.
    startMs: resolveMs(d.startText, d.startMs),
    endMs: resolveMs(d.endText, d.endMs),
    ...(d.name ? { name: d.name } : {}),
    ...(d.unsplittable ? { unsplittable: d.unsplittable } : {}),
    ...(d.artwork ? { artwork: d.artwork } : {}),
    ...(d.tags ? { tags: d.tags } : {}),
    ...(d.language ? { language: d.language } : {}),
    ...(d.holdReason
      ? { attention: `Needs a closer look: ${d.holdReason}` }
      : d.unsplittable
        ? { attention: "Loomarr may have missed a cut here." }
        : {}),
  }));

  return (
    <div className={cn("flex flex-col gap-4", className)}>
      {/* The reel at a glance, above the rows it describes (the v2 mock's `rl.strip`). Clicking
          a block focuses that segment's row — the strip is a map, the rows are the work. */}
      <SegmentFilmstrip
        segments={stripSegments}
        {...(focusedKey ? { activeKey: focusedKey } : {})}
        onSelect={(key) => {
          setFocusedKey(key);
          setPreviewKey(key);
        }}
      />

      {draft.map((seg, i) => (
        <SegmentRow
          key={seg.key}
          segment={seg}
          position={i}
          last={i === draft.length - 1}
          focused={focusedKey === seg.key}
          clipHash={proposal.clipHash}
          minClipDurationMs={minClipDurationMs}
          // ⚠ ONE open at a time, held here rather than per row. Two expanded previews are two
          // audio streams talking over each other, and a per-row `useState` would let all 52
          // open — 52 range requests against one 20-minute file.
          previewOpen={previewKey === seg.key}
          onPreviewChange={(open) => setPreviewKey(open ? seg.key : undefined)}
          onChange={(patch) => setSegment(i, patch)}
          onDrop={() => drop(i)}
          onMergeWithNext={() => mergeWithNext(i)}
        />
      ))}

      {draft.length === 0 && (
        <p className="text-muted-foreground text-sm">
          You removed every clip. Go back to keep the original recording, or restore a clip before continuing.
        </p>
      )}

      <div className="flex items-center justify-between gap-3 border-border border-t pt-4">
        <Button variant="outline" size="sm" onClick={onBack}>
          Back
        </Button>
        <div className="flex items-center gap-3">
          <span className="font-mono text-muted-foreground text-sm tabular-nums">
            {pluralize(draft.length, "clip")}
          </span>
          <Button size="sm" disabled={!confirmable || confirming} onClick={() => onConfirm(toWire(draft))}>
            {confirming ? "Keeping clips…" : "Keep clips"}
          </Button>
        </div>
      </div>
    </div>
  );
};

interface SegmentRowProps {
  segment: DraftSegment;
  position: number;
  last: boolean;
  focused: boolean;
  // The COMPOSITE's hash — what the preview plays a window of. A proposed cut has no bytes yet.
  clipHash: string;
  minClipDurationMs?: number;
  previewOpen: boolean;
  onPreviewChange: (open: boolean) => void;
  onChange: (patch: Partial<DraftSegment>) => void;
  onDrop: () => void;
  onMergeWithNext: () => void;
}

const SegmentRow = ({
  segment,
  position,
  last,
  focused,
  clipHash,
  minClipDurationMs,
  previewOpen,
  onPreviewChange,
  onChange,
  onDrop,
  onMergeWithNext,
}: SegmentRowProps) => {
  const n = position + 1;
  const span = spanMs(segment);
  const valid = isValid(segment);
  const languageMessage = unresolvedLanguageMessage(segment);
  const namingMessage = nameDetail(segment);
  const timingNeedsAttention =
    !valid ||
    segment.unsplittable ||
    (span !== undefined && minClipDurationMs !== undefined && span < minClipDurationMs);
  const hasDetails = Boolean(
    (!segment.language && languageMessage) ||
      segment.dupOf ||
      segment.boundaryConfidence ||
      segment.transcript,
  );
  const ref = useRef<HTMLDivElement>(null);

  // ⚠ Clicking a strip block has to SHOW the row, not merely tint it. A long reel puts most of
  // its segments off-screen, so a highlight the operator has to go hunting for is the same as no
  // response at all. `block: "nearest"` avoids yanking the page when the row is already visible.
  useEffect(() => {
    if (focused) ref.current?.scrollIntoView({ behavior: "smooth", block: "nearest" });
  }, [focused]);

  return (
    <Card ref={ref} className={cn(focused && "ring-1 ring-signal-300")}>
      <section aria-label={`Segment ${n}: ${segment.name || "unnamed"}`} className="flex flex-col gap-3 p-3">
        <div className="flex flex-wrap items-start gap-3">
          <SegmentPreview
            clipHash={clipHash}
            // Preview the draft the operator can actually keep. Untouched cuts retain detector
            // millisecond precision; typed values use the same resolver as the confirm body.
            startMs={resolveMs(segment.startText, segment.startMs)}
            endMs={resolveMs(segment.endText, segment.endMs)}
            position={position}
            labelledBy={`seg-num-${position}`}
            artwork={segment.artwork}
            open={previewOpen}
            onOpenChange={onPreviewChange}
            autoPlay
          />
          <div className="min-w-44 flex-1 pt-0.5">
            <div className="flex items-center gap-2">
              <span
                id={`seg-num-${position}`}
                className="font-mono text-muted-foreground text-xs tabular-nums"
              >
                #{n}
              </span>
              <h3 className="truncate font-medium text-sm">{segment.name || "Unnamed clip"}</h3>
            </div>
            <p
              className={cn(
                "mt-1 font-mono text-xs tabular-nums",
                valid ? "text-muted-foreground" : "text-onair-300",
              )}
              title={valid ? undefined : "Needs mm:ss times, end after start, at least 3 seconds"}
            >
              {`${segment.startText}–${segment.endText} · ${span !== undefined && span > 0 ? formatClipDuration(span) : "invalid span"}`}
            </p>
            <div className="mt-2 flex flex-wrap items-center gap-1.5">
              {segment.era ? <Badge variant="neutral">{`${segment.era}s`}</Badge> : null}
              {segment.audience ? (
                <Badge variant="neutral">{AUDIENCE_LABEL[segment.audience] ?? segment.audience}</Badge>
              ) : null}
              {segment.category ? <Badge variant="neutral">{segment.category}</Badge> : null}
              {segment.languageChecked && segment.language ? (
                <Badge variant="neutral">
                  {segment.language === "none" ? "No speech" : languageName(segment.language)}
                </Badge>
              ) : null}
              {(() => {
                const extra = (segment.tags ?? []).filter((tag) => tag !== segment.category).length;
                return extra > 0 ? <Badge variant="neutral">+{extra}</Badge> : null;
              })()}
            </div>
          </div>
          <div className="ml-auto flex flex-wrap justify-end gap-1">
            {!last && (
              <Button variant="ghost" size="sm" onClick={onMergeWithNext} title="Join this clip and the next">
                <Merge aria-hidden />
                Join next
              </Button>
            )}
            <Button variant="ghost" size="sm" onClick={onDrop} title="Do not keep this clip">
              <Trash2 aria-hidden />
              Remove
            </Button>
          </div>
        </div>

        {segment.holdReason ? (
          <p role="status" className="rounded-sm bg-caution-tint-15 px-2 py-1.5 text-caution text-sm">
            {`Take a closer look: ${segment.holdReason}.`}
          </p>
        ) : null}
        {segment.unsplittable ? (
          <p className="rounded-sm bg-onair-tint-15 px-2 py-1.5 text-onair-300 text-sm">
            Loomarr may have missed a cut here. Preview it, then adjust the timing, join it with the next
            clip, or remove it.
          </p>
        ) : null}
        {span !== undefined && minClipDurationMs !== undefined && span < minClipDurationMs ? (
          <p role="status" className="rounded-sm bg-onair-tint-15 px-2 py-1.5 text-onair-300 text-sm">
            This clip is shorter than your {formatClipDuration(minClipDurationMs)} minimum. Make it longer,
            join it with the next clip, or remove it.
          </p>
        ) : null}

        {segment.suggestedEra ? (
          <div className="flex flex-wrap items-center gap-2 text-sm">
            <span>Loomarr thinks this may be from around {segment.suggestedEra}.</span>
            <Button
              variant="outline"
              size="sm"
              onClick={() => onChange({ era: segment.suggestedEra, suggestedEra: undefined })}
            >
              {`Use ${segment.suggestedEra}`}
            </Button>
            <Button variant="ghost" size="sm" onClick={() => onChange({ suggestedEra: undefined })}>
              Not right
            </Button>
          </div>
        ) : null}

        <details open={timingNeedsAttention} className="rounded-md border border-border/70 px-3 py-2">
          <summary className="flex cursor-pointer list-none items-center gap-2 font-medium text-sm">
            <Pencil aria-hidden className="size-3.5" /> Rename or adjust timing
          </summary>
          <div className="mt-3 grid gap-3 sm:grid-cols-[minmax(12rem,1fr)_7rem_7rem]">
            <div>
              <Label htmlFor={`seg-name-${position}`}>Name</Label>
              <Input
                id={`seg-name-${position}`}
                value={segment.name}
                onChange={(event) => onChange({ name: event.target.value, ...nameNowOwnedByOperator })}
              />
              {namingMessage ? <p className="mt-1.5 text-muted-foreground text-xs">{namingMessage}</p> : null}
            </div>
            <div>
              <Label htmlFor={`seg-start-${position}`}>Starts</Label>
              <Input
                id={`seg-start-${position}`}
                aria-label="Start (mm:ss)"
                className="font-mono tabular-nums"
                value={segment.startText}
                onChange={(event) =>
                  onChange({
                    startText: event.target.value,
                    artwork: undefined,
                    ...languageNeedsRecheck,
                    ...nameNowOwnedByOperator,
                  })
                }
              />
            </div>
            <div>
              <Label htmlFor={`seg-end-${position}`}>Ends</Label>
              <Input
                id={`seg-end-${position}`}
                aria-label="End (mm:ss)"
                className="font-mono tabular-nums"
                value={segment.endText}
                onChange={(event) =>
                  onChange({
                    endText: event.target.value,
                    artwork: undefined,
                    ...languageNeedsRecheck,
                    ...nameNowOwnedByOperator,
                  })
                }
              />
            </div>
          </div>
        </details>

        {hasDetails ? (
          <details className="px-1 text-sm">
            <summary className="cursor-pointer text-muted-foreground">Details</summary>
            <div className="mt-2 space-y-2 text-muted-foreground">
              {!segment.language && languageMessage ? (
                <p title={segment.languageNote}>{languageMessage}</p>
              ) : null}
              {segment.dupOf ? <p>{`Looks like a clip already in your library: ${segment.dupOf}`}</p> : null}
              {segment.boundaryConfidence ? (
                <p className="font-mono text-xs">
                  {`Cut confidence ${segment.boundaryConfidence}%`}
                  {segment.startEvidence ? ` · start: ${segment.startEvidence}` : ""}
                  {segment.endEvidence ? ` · end: ${segment.endEvidence}` : ""}
                </p>
              ) : null}
              {segment.transcript ? (
                <pre className="max-h-48 overflow-auto whitespace-pre-wrap rounded-sm bg-static-800 p-3 font-mono text-xs">
                  {segment.transcript}
                </pre>
              ) : null}
            </div>
          </details>
        ) : null}
      </section>
    </Card>
  );
};

export { SplitReviewEditor };
