import type { SplitSegment } from "@loomarr/api";
import { formatClipDuration, formatMmSs, parseMmSs, pluralize } from "@loomarr/core";
import { ChevronDown, ChevronRight, Merge, Trash2 } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { Badge, Button, Card, Input, Label } from "@/components/ui";
import { cn } from "@/lib";
import { SegmentFilmstrip } from "../segment-filmstrip";
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
interface DraftSegment extends SplitSegment {
  key: string;
  startText: string;
  endText: string;
}

const toDraft = (seg: SplitSegment): DraftSegment => ({
  ...seg,
  key: `seg-${seg.index}-${seg.startMs}`,
  startText: formatMmSs(seg.startMs),
  endText: formatMmSs(seg.endMs),
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
    const { key: _k, startText: _s, endText: _e, ...seg } = d;
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

const SplitReviewEditor = ({
  proposal,
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
  }));

  return (
    <div className={cn("flex flex-col gap-4", className)}>
      {/* The reel at a glance, above the rows it describes (the v2 mock's `rl.strip`). Clicking
          a block focuses that segment's row — the strip is a map, the rows are the work. */}
      <SegmentFilmstrip
        segments={stripSegments}
        {...(focusedKey ? { activeKey: focusedKey } : {})}
        onFocus={setFocusedKey}
      />

      {draft.map((seg, i) => (
        <SegmentRow
          key={seg.key}
          segment={seg}
          position={i}
          last={i === draft.length - 1}
          focused={focusedKey === seg.key}
          onChange={(patch) => setSegment(i, patch)}
          onDrop={() => drop(i)}
          onMergeWithNext={() => mergeWithNext(i)}
        />
      ))}

      {draft.length === 0 && (
        <p className="text-muted-foreground text-sm">
          Every segment has been dropped. Go back to keep the compilation whole. Confirming an empty cut list
          is not a thing Loomarr will do.
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
            {confirming ? "Cutting…" : "Confirm cuts"}
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
  onChange: (patch: Partial<DraftSegment>) => void;
  onDrop: () => void;
  onMergeWithNext: () => void;
}

const SegmentRow = ({
  segment,
  position,
  last,
  focused,
  onChange,
  onDrop,
  onMergeWithNext,
}: SegmentRowProps) => {
  const [showTranscript, setShowTranscript] = useState(false);
  const n = position + 1;
  const span = spanMs(segment);
  const valid = isValid(segment);
  const ref = useRef<HTMLDivElement>(null);

  // ⚠ Clicking a strip block has to SHOW the row, not merely tint it. A long reel puts most of
  // its segments off-screen, so a highlight the operator has to go hunting for is the same as no
  // response at all. `block: "nearest"` avoids yanking the page when the row is already visible.
  useEffect(() => {
    if (focused) ref.current?.scrollIntoView({ behavior: "smooth", block: "nearest" });
  }, [focused]);

  return (
    <Card ref={ref} className={cn(focused && "ring-1 ring-signal-300")}>
      <section aria-label={`Segment ${n}: ${segment.name || "unnamed"}`} className="flex flex-col gap-3 p-4">
        <div className="flex flex-wrap items-end gap-3">
          <span className="font-mono text-muted-foreground text-sm tabular-nums">#{n}</span>
          <div className="min-w-48 flex-1">
            <Label htmlFor={`seg-name-${position}`}>Name</Label>
            <Input
              id={`seg-name-${position}`}
              value={segment.name}
              onChange={(e) => onChange({ name: e.target.value })}
            />
          </div>
          <div>
            <Label htmlFor={`seg-start-${position}`}>Start (mm:ss)</Label>
            <Input
              id={`seg-start-${position}`}
              className="w-24 font-mono tabular-nums"
              value={segment.startText}
              onChange={(e) => onChange({ startText: e.target.value })}
            />
          </div>
          <div>
            <Label htmlFor={`seg-end-${position}`}>End (mm:ss)</Label>
            <Input
              id={`seg-end-${position}`}
              className="w-24 font-mono tabular-nums"
              value={segment.endText}
              onChange={(e) => onChange({ endText: e.target.value })}
            />
          </div>
          <span
            className={cn(
              "font-mono text-sm tabular-nums",
              valid ? "text-muted-foreground" : "text-onair-300",
            )}
            // An inverted or unparseable span is a certainty to say out loud, not a 422 to
            // discover after the fact — the operator is editing CUT POINTS, the one thing
            // this screen exists to get right.
            title={valid ? undefined : "Needs mm:ss times, end after start, at least 3 seconds"}
          >
            {span !== undefined && span > 0 ? formatClipDuration(span) : "invalid span"}
          </span>
          <div className="ml-auto flex gap-2">
            {!last && (
              <Button
                variant="ghost"
                size="sm"
                onClick={onMergeWithNext}
                title="Join this segment and the next into one span"
              >
                <Merge aria-hidden />
                Merge with next
              </Button>
            )}
            <Button variant="ghost" size="sm" onClick={onDrop} title="Remove this segment from the cut list">
              <Trash2 aria-hidden />
              Drop
            </Button>
          </div>
        </div>

        <div className="flex flex-wrap items-center gap-1.5">
          {segment.era ? <Badge variant="neutral">{`${segment.era}s`}</Badge> : null}
          {segment.audience ? (
            <Badge variant="neutral">{AUDIENCE_LABEL[segment.audience] ?? segment.audience}</Badge>
          ) : null}
          {/* Tags (§10 V45a), same read-only rendering as ClipCard: the headline badge is the
              derived primary product leaf (`category`); a "+N" chip signals more taxonomy tags
              exist without listing the full rollup set. No inline cycle — a segment's tags ride
              along from detection/grounding, not something this review gate edits directly. */}
          {segment.category ? <Badge variant="neutral">{segment.category}</Badge> : null}
          {(() => {
            const extra = (segment.tags ?? []).filter((t) => t !== segment.category).length;
            return extra > 0 ? (
              <Badge variant="neutral" title="This segment has more tags">
                +{extra}
              </Badge>
            ) : null;
          })()}

          {/* An unconfirmed era (§10 grounding): the classifier guessed a year that appears
              in NO text signal. Accept grounds it as the operator's tag; reject drops the
              guess. Neither is default — the question stays open until a human answers. */}
          {segment.suggestedEra ? (
            <span className="inline-flex items-center gap-1.5">
              <Badge variant="suggest" title="AI guess. The year isn't in the transcript or source text">
                {`${segment.suggestedEra}s?`}
              </Badge>
              <Button
                variant="outline"
                size="sm"
                onClick={() => onChange({ era: segment.suggestedEra, suggestedEra: undefined })}
              >
                {`Accept ${segment.suggestedEra}`}
              </Button>
              <Button variant="ghost" size="sm" onClick={() => onChange({ suggestedEra: undefined })}>
                Reject
              </Button>
            </span>
          ) : null}

          {/* dHash duplicate (§10 step 5): a FLAG, never a silent drop. The operator decides —
              usually by dropping this segment. */}
          {segment.dupOf ? (
            <Badge variant="caution" title="This looks like a clip already in your catalog">
              {`Already in the catalog: ${segment.dupOf}`}
            </Badge>
          ) : null}
        </div>

        {/* Unsplittable: over-long AND the rescue could not see boundaries (no whisper, or
            none detectable in the text). Said unmistakably, because the alternative is
            guessing — exactly what the era rule forbids in tag form. */}
        {segment.unsplittable ? (
          <p className="rounded-sm bg-onair-tint-15 px-2 py-1.5 text-onair-300 text-sm">
            Loomarr couldn't see boundaries in this span, either because there's no transcript or because
            there are no detectable breaks. Cut it by hand with the times above, or drop it.
          </p>
        ) : null}

        {segment.transcript ? (
          <div>
            <Button
              variant="ghost"
              size="sm"
              aria-expanded={showTranscript}
              onClick={() => setShowTranscript((v) => !v)}
            >
              {showTranscript ? <ChevronDown aria-hidden /> : <ChevronRight aria-hidden />}
              Transcript
            </Button>
            {showTranscript && (
              <pre className="mt-1 max-h-48 overflow-auto whitespace-pre-wrap rounded-sm bg-static-800 p-3 font-mono text-muted-foreground text-xs">
                {segment.transcript}
              </pre>
            )}
          </div>
        ) : null}
      </section>
    </Card>
  );
};

export { SplitReviewEditor };
