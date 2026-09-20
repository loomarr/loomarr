import type { SplitReviewSegmentDTO } from "@loomarr/api/models/splitReviewSegmentDTO";
import type { SplitSegment } from "@loomarr/api/models/splitSegment";
import { formatClipDuration, formatMmSs, parseMmSs, pluralize } from "@loomarr/core/format";
import { ChevronLeft, ChevronRight, Merge, Pencil, Trash2 } from "lucide-react";
import { useRef, useState } from "react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Image } from "@/components/ui/image";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet";
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
  // Selection belongs to this review session, not the URL: merges and removals deliberately change
  // the draft identities beneath it. The trigger is retained separately so Base UI restores focus
  // to the exact filmstrip tile or compact row that opened the Sheet.
  const [selectedKey, setSelectedKey] = useState<string>();
  const detailTrigger = useRef<HTMLElement>(null);

  const openSegment = (key: string, trigger: HTMLElement) => {
    detailTrigger.current = trigger;
    setSelectedKey(key);
  };

  const setSegment = (i: number, patch: Partial<DraftSegment>) =>
    setDraft((prev) => prev.map((d, j) => (j === i ? { ...d, ...patch } : d)));

  const drop = (i: number) => {
    const removed = draft[i];
    const next = draft[i + 1] ?? draft[i - 1];
    setDraft((prev) => prev.filter((_, j) => j !== i));
    if (removed?.key === selectedKey) setSelectedKey(next?.key);
  };

  // Merge with next CONCATENATES THE SPANS (end becomes the next segment's end). Tags keep
  // the first segment's, inheriting from the next only what the first lacks; transcripts
  // join so the reviewer keeps the evidence for the new, longer span.
  // ⚠ `category` (kept here as first-wins, matching every other single-value field) is a
  // DERIVED shadow (§10 V45a) — this review gate never writes it directly, so first-wins is
  // just which half's shadow happens to display until the confirmed segment is re-tagged.
  const mergeWithNext = (i: number) => {
    const a = draft[i];
    const b = draft[i + 1];
    if (!a || !b) return;
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
    setDraft((prev) => [...prev.slice(0, i), merged, ...prev.slice(i + 2)]);
    setSelectedKey(merged.key);
  };

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
  const selectedIndex = draft.findIndex((segment) => segment.key === selectedKey);
  const selected = selectedIndex >= 0 ? draft[selectedIndex] : undefined;

  return (
    <div className={cn("flex w-full min-w-0 max-w-full flex-col gap-4 overflow-x-hidden", className)}>
      {/* The reel stays stable while the selected clip is inspected in the shared Sheet. */}
      <SegmentFilmstrip
        segments={stripSegments}
        {...(selectedKey ? { activeKey: selectedKey } : {})}
        onSelect={openSegment}
      />

      <ol className="flex list-none flex-col gap-2" aria-label="Clips in this recording">
        {draft.map((seg, i) => (
          <SegmentSummaryRow
            key={seg.key}
            segment={seg}
            position={i}
            minClipDurationMs={minClipDurationMs}
            selected={selectedKey === seg.key}
            onOpen={(trigger) => openSegment(seg.key, trigger)}
          />
        ))}
      </ol>

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

      <Sheet
        open={Boolean(selected)}
        onOpenChange={(open) => {
          if (!open) setSelectedKey(undefined);
        }}
        swipeDirection="right"
      >
        {selected ? (
          <SheetContent finalFocus={detailTrigger}>
            <SheetHeader>
              <SheetTitle>{selected.name || "Unnamed clip"}</SheetTitle>
              <SheetDescription>
                {`Clip ${selectedIndex + 1} of ${draft.length} · ${selected.startText}–${selected.endText}`}
              </SheetDescription>
            </SheetHeader>
            <div className="flex flex-col gap-5 p-6">
              <div className="flex items-center justify-between gap-3">
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  disabled={selectedIndex === 0}
                  onClick={() => setSelectedKey(draft[selectedIndex - 1]?.key)}
                  aria-label="Previous clip"
                >
                  <ChevronLeft aria-hidden />
                  Previous
                </Button>
                <span className="font-mono text-muted-foreground text-xs tabular-nums">
                  {selectedIndex + 1} / {draft.length}
                </span>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  disabled={selectedIndex === draft.length - 1}
                  onClick={() => setSelectedKey(draft[selectedIndex + 1]?.key)}
                  aria-label="Next clip"
                >
                  Next
                  <ChevronRight aria-hidden />
                </Button>
              </div>

              <SegmentInspector
                key={selected.key}
                segment={selected}
                position={selectedIndex}
                last={selectedIndex === draft.length - 1}
                clipHash={proposal.clipHash}
                minClipDurationMs={minClipDurationMs}
                onChange={(patch) => setSegment(selectedIndex, patch)}
                onDrop={() => drop(selectedIndex)}
                onMergeWithNext={() => mergeWithNext(selectedIndex)}
              />
            </div>
          </SheetContent>
        ) : null}
      </Sheet>
    </div>
  );
};

interface SegmentSummaryRowProps {
  segment: DraftSegment;
  position: number;
  minClipDurationMs?: number;
  selected: boolean;
  onOpen: (trigger: HTMLButtonElement) => void;
}

const SegmentSummaryRow = ({
  segment,
  position,
  minClipDurationMs,
  selected,
  onOpen,
}: SegmentSummaryRowProps) => {
  const n = position + 1;
  const span = spanMs(segment);
  const valid = isValid(segment);
  const needsAttention =
    !valid ||
    segment.unsplittable ||
    Boolean(segment.holdReason) ||
    (span !== undefined && minClipDurationMs !== undefined && span < minClipDurationMs);

  return (
    <li>
      <Card className={cn(selected && "ring-1 ring-signal-300")}>
        <button
          type="button"
          aria-label={`Open clip ${n}: ${segment.name || "unnamed"}`}
          aria-current={selected ? "true" : undefined}
          onClick={(event) => onOpen(event.currentTarget)}
          className="flex w-full items-center gap-3 rounded-lg p-3 text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          <span className="relative h-14 w-24 shrink-0 overflow-hidden rounded-md border border-border bg-static-900">
            {segment.artwork ? (
              <Image image={segment.artwork} alt="" sizes="96px" className="size-full object-cover" />
            ) : (
              <span className="flex size-full items-center justify-center px-2 text-center text-[10px] text-muted-foreground">
                Preview unavailable
              </span>
            )}
          </span>
          <span className="min-w-0 flex-1">
            <span className="flex items-center gap-2">
              <span className="font-mono text-muted-foreground text-xs tabular-nums">#{n}</span>
              <span className="truncate font-medium text-sm">{segment.name || "Unnamed clip"}</span>
            </span>
            <span
              className={cn(
                "mt-1 block font-mono text-xs tabular-nums",
                valid ? "text-muted-foreground" : "text-onair-300",
              )}
            >
              {`${segment.startText}–${segment.endText} · ${span !== undefined && span > 0 ? formatClipDuration(span) : "invalid span"}`}
            </span>
            <span className="mt-1.5 flex flex-wrap items-center gap-1.5">
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
              {needsAttention ? <Badge variant="caution">Needs a closer look</Badge> : null}
            </span>
          </span>
          <ChevronRight aria-hidden className="size-4 shrink-0 text-muted-foreground" />
        </button>
      </Card>
    </li>
  );
};

interface SegmentInspectorProps {
  segment: DraftSegment;
  position: number;
  last: boolean;
  // The COMPOSITE's hash — what the preview plays a window of. A proposed cut has no bytes yet.
  clipHash: string;
  minClipDurationMs?: number;
  onChange: (patch: Partial<DraftSegment>) => void;
  onDrop: () => void;
  onMergeWithNext: () => void;
}

const SegmentInspector = ({
  segment,
  position,
  last,
  clipHash,
  minClipDurationMs,
  onChange,
  onDrop,
  onMergeWithNext,
}: SegmentInspectorProps) => {
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
    namingMessage ||
      (!segment.language && languageMessage) ||
      segment.dupOf ||
      segment.boundaryConfidence ||
      segment.transcript,
  );
  return (
    <section aria-label={`Clip ${n}: ${segment.name || "unnamed"}`} className="flex flex-col gap-4">
      <SegmentPreview
        clipHash={clipHash}
        // Preview the draft the operator can actually keep. Untouched cuts retain detector
        // millisecond precision; typed values use the same resolver as the confirm body.
        startMs={resolveMs(segment.startText, segment.startMs)}
        endMs={resolveMs(segment.endText, segment.endMs)}
        position={position}
        labelledBy={`seg-num-${position}`}
        artwork={segment.artwork}
        open
        onOpenChange={() => {}}
        autoPlay
        embedded
      />

      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0 flex-1">
          <p
            className={cn(
              "font-mono text-xs tabular-nums",
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
        <div className="flex flex-wrap justify-end gap-1">
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
          Loomarr may have missed a cut here. Preview it, then adjust the timing, join it with the next clip,
          or remove it.
        </p>
      ) : null}
      {span !== undefined && minClipDurationMs !== undefined && span < minClipDurationMs ? (
        <p role="status" className="rounded-sm bg-onair-tint-15 px-2 py-1.5 text-onair-300 text-sm">
          This clip is shorter than your {formatClipDuration(minClipDurationMs)} minimum. Make it longer, join
          it with the next clip, or remove it.
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
            {namingMessage ? <p>{namingMessage}</p> : null}
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
  );
};

export { SplitReviewEditor };
