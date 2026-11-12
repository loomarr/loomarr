import type { SplitReviewSegmentDTO } from "@loomarr/api/models/splitReviewSegmentDTO";
import type { SplitSegment } from "@loomarr/api/models/splitSegment";
import { formatClipDuration, formatMmSs, parseMmSs, pluralize } from "@loomarr/core/format";
import {
  Action,
  Badge,
  Card,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  Field,
  Icon,
  icons,
  Stack,
  StatusLabel,
  Text,
  Tooltip,
} from "@loomarr/design-system";
import { useRef, useState } from "react";
import { ServedImage } from "@/components/loomarr/served-image";
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
  // to the exact filmstrip tile or compact row that opened the panel.
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
    <div className={cn("flex w-full max-w-full min-w-0 flex-col gap-4 overflow-x-hidden", className)}>
      {/* The reel stays stable while the selected clip is inspected in the side panel. */}
      <SegmentFilmstrip
        segments={stripSegments}
        {...(selectedKey ? { activeKey: selectedKey } : {})}
        onSelect={openSegment}
      />

      <Stack aria-label="Clips in this recording" className="list-none" gap="inline" render="ol">
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
      </Stack>

      {draft.length === 0 && (
        <Text as="p" textRole="compact" tone="secondary">
          You removed every clip. Go back to keep the original recording, or restore a clip before continuing.
        </Text>
      )}

      <Stack
        align="center"
        className="border-border border-t pt-4"
        direction="row"
        gap="control"
        justify="between"
      >
        <Action onPress={onBack} variant="outline">
          Back
        </Action>
        <Stack align="center" direction="row" gap="control">
          <Text as="span" className="tabular-nums" textRole="mono">
            {pluralize(draft.length, "clip")}
          </Text>
          <Action disabled={!confirmable || confirming} onPress={() => onConfirm(toWire(draft))}>
            {confirming ? "Keeping clips…" : "Keep clips"}
          </Action>
        </Stack>
      </Stack>

      <Dialog
        open={Boolean(selected)}
        onOpenChange={(open) => {
          if (!open) setSelectedKey(undefined);
        }}
      >
        {selected ? (
          <DialogContent finalFocus={detailTrigger} placement="side">
            <DialogHeader>
              <DialogTitle>{selected.name || "Unnamed clip"}</DialogTitle>
              <DialogDescription>
                {`Clip ${selectedIndex + 1} of ${draft.length} · ${selected.startText}–${selected.endText}`}
              </DialogDescription>
            </DialogHeader>
            <Stack className="p-6" gap="5">
              <Stack align="center" direction="row" gap="control" justify="between">
                <Action
                  accessibilityLabel="Previous clip"
                  disabled={selectedIndex === 0}
                  icon="earlier"
                  onPress={() => setSelectedKey(draft[selectedIndex - 1]?.key)}
                  variant="outline"
                >
                  Previous
                </Action>
                <Text as="span" className="tabular-nums" textRole="mono">
                  {selectedIndex + 1} / {draft.length}
                </Text>
                <Action
                  accessibilityLabel="Next clip"
                  disabled={selectedIndex === draft.length - 1}
                  icon="later"
                  iconPlacement="end"
                  onPress={() => setSelectedKey(draft[selectedIndex + 1]?.key)}
                  variant="outline"
                >
                  Next
                </Action>
              </Stack>

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
            </Stack>
          </DialogContent>
        ) : null}
      </Dialog>
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
      {/* Selection is neutral (#1909): amber is for focus and the primary action. */}
      <Card className={cn(selected && "ring-foreground ring-1")}>
        <button
          type="button"
          aria-current={selected ? "true" : undefined}
          onClick={(event) => onOpen(event.currentTarget)}
          className="focus-visible:ring-ring relative flex w-full items-center gap-3 rounded-lg p-3 text-left focus-visible:ring-2 focus-visible:outline-none"
        >
          {/* The name comes from the content, so it holds every visible word (WCAG 2.5.3). */}
          <span className="sr-only">Open clip</span>{" "}
          <span className="border-border bg-static-900 relative h-14 w-24 shrink-0 overflow-hidden rounded-md border">
            {segment.artwork ? (
              <ServedImage image={segment.artwork} alt="" sizes="96px" />
            ) : (
              <Stack
                align="center"
                className="text-muted-foreground size-full px-2 text-center text-[10px]"
                direction="row"
                justify="center"
                render="span"
              >
                Preview unavailable
              </Stack>
            )}
          </span>
          <span className="min-w-0 flex-1">
            <Stack align="center" direction="row" gap="inline" render="span">
              <Text as="span" className="tabular-nums" textRole="mono">
                #{n}
              </Text>
              <Text as="span" className="truncate" textRole="cardLabel" tone="primary">
                {segment.name || "Unnamed clip"}
              </Text>
            </Stack>
            <Text
              as="span"
              className="mt-1 block tabular-nums"
              textRole="mono"
              tone={valid ? "secondary" : "danger"}
            >
              {`${segment.startText}–${segment.endText} · ${span !== undefined && span > 0 ? formatClipDuration(span) : "invalid span"}`}
            </Text>
            <Stack align="center" className="mt-1.5" direction="row" gap="1.5" render="span" wrap>
              {segment.era ? <Badge shape="square" tone="neutral">{`${segment.era}s`}</Badge> : null}
              {segment.audience ? (
                <Badge shape="square" tone="neutral">
                  {AUDIENCE_LABEL[segment.audience] ?? segment.audience}
                </Badge>
              ) : null}
              {segment.category ? (
                <Badge shape="square" tone="neutral">
                  {segment.category}
                </Badge>
              ) : null}
              {segment.languageChecked && segment.language ? (
                <Badge shape="square" tone="neutral">
                  {segment.language === "none" ? "No speech" : languageName(segment.language)}
                </Badge>
              ) : null}
              {needsAttention ? <StatusLabel tone="warn">Needs a closer look</StatusLabel> : null}
            </Stack>
          </span>
          <Icon decorative glyph={icons.rowExpand} size="compact" tone="muted" />
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
    <Stack aria-label={`Clip ${n}: ${segment.name || "unnamed"}`} gap="4" render="section">
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

      <Stack align="start" direction="row" gap="control" justify="between" wrap>
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
          <Stack align="center" className="mt-2" direction="row" gap="1.5" wrap>
            {segment.era ? <Badge shape="square" tone="neutral">{`${segment.era}s`}</Badge> : null}
            {segment.audience ? (
              <Badge shape="square" tone="neutral">
                {AUDIENCE_LABEL[segment.audience] ?? segment.audience}
              </Badge>
            ) : null}
            {segment.category ? (
              <Badge shape="square" tone="neutral">
                {segment.category}
              </Badge>
            ) : null}
            {segment.languageChecked && segment.language ? (
              <Badge shape="square" tone="neutral">
                {segment.language === "none" ? "No speech" : languageName(segment.language)}
              </Badge>
            ) : null}
            {(() => {
              const extra = (segment.tags ?? []).filter((tag) => tag !== segment.category).length;
              return extra > 0 ? (
                <Badge shape="square" tone="neutral">
                  +{extra}
                </Badge>
              ) : null;
            })()}
          </Stack>
        </div>
        <Stack direction="row" gap="1" justify="end" wrap>
          {!last && (
            <Tooltip content="Join this clip and the next">
              <Action onPress={onMergeWithNext} variant="ghost">
                Join next
              </Action>
            </Tooltip>
          )}
          <Tooltip content="Do not keep this clip">
            <Action icon="delete" onPress={onDrop} variant="ghost">
              Remove
            </Action>
          </Tooltip>
        </Stack>
      </Stack>

      {segment.holdReason ? (
        <p role="status" className="bg-caution-tint-15 text-caution rounded-sm px-2 py-1.5 text-sm">
          {`Take a closer look: ${segment.holdReason}.`}
        </p>
      ) : null}
      {segment.unsplittable ? (
        <Text as="p" className="bg-onair-tint-15 rounded-sm px-2 py-1.5" textRole="compact" tone="danger">
          Loomarr may have missed a cut here. Preview it, then adjust the timing, join it with the next clip,
          or remove it.
        </Text>
      ) : null}
      {span !== undefined && minClipDurationMs !== undefined && span < minClipDurationMs ? (
        <p role="status" className="bg-onair-tint-15 text-onair-300 rounded-sm px-2 py-1.5 text-sm">
          This clip is shorter than your {formatClipDuration(minClipDurationMs)} minimum. Make it longer, join
          it with the next clip, or remove it.
        </p>
      ) : null}

      {segment.suggestedEra ? (
        <Stack align="center" direction="row" gap="inline" wrap>
          <Text as="span" textRole="compact" tone="primary">
            Loomarr thinks this may be from around {segment.suggestedEra}.
          </Text>
          <Action
            onPress={() => onChange({ era: segment.suggestedEra, suggestedEra: undefined })}
            variant="outline"
          >
            {`Use ${segment.suggestedEra}`}
          </Action>
          <Action onPress={() => onChange({ suggestedEra: undefined })} variant="ghost">
            Not right
          </Action>
        </Stack>
      ) : null}

      <details open={timingNeedsAttention} className="border-border/70 rounded-md border px-3 py-2">
        <Text
          as="summary"
          className="flex cursor-pointer list-none items-center gap-2"
          textRole="cardLabel"
          tone="primary"
        >
          <Icon decorative glyph={icons.edit} size="compact" tone="inherit" /> Rename or adjust timing
        </Text>
        <div className="mt-3 grid gap-3 sm:grid-cols-[minmax(12rem,1fr)_7rem_7rem]">
          <Field
            id={`seg-name-${position}`}
            label="Name"
            onChangeText={(name) => onChange({ name, ...nameNowOwnedByOperator })}
            value={segment.name}
          />
          <Field
            accessibilityLabel="Start (mm:ss)"
            id={`seg-start-${position}`}
            keyboardType="numbers-and-punctuation"
            label="Starts"
            onChangeText={(startText) =>
              onChange({ startText, artwork: undefined, ...languageNeedsRecheck, ...nameNowOwnedByOperator })
            }
            value={segment.startText}
          />
          <Field
            accessibilityLabel="End (mm:ss)"
            id={`seg-end-${position}`}
            keyboardType="numbers-and-punctuation"
            label="Ends"
            onChangeText={(endText) =>
              onChange({ endText, artwork: undefined, ...languageNeedsRecheck, ...nameNowOwnedByOperator })
            }
            value={segment.endText}
          />
        </div>
      </details>

      {hasDetails ? (
        <details className="px-1">
          <Text as="summary" className="cursor-pointer" textRole="compact" tone="secondary">
            Details
          </Text>
          <div className="mt-2 space-y-2">
            {namingMessage ? (
              <Text as="p" textRole="compact" tone="secondary">
                {namingMessage}
              </Text>
            ) : null}
            {!segment.language && languageMessage ? (
              <p className="text-muted-foreground text-sm" title={segment.languageNote}>
                {languageMessage}
              </p>
            ) : null}
            {segment.dupOf ? (
              <Text as="p" textRole="compact" tone="secondary">
                {`Looks like a clip already in your library: ${segment.dupOf}`}
              </Text>
            ) : null}
            {segment.boundaryConfidence ? (
              <Text as="p" textRole="mono">
                {`Cut confidence ${segment.boundaryConfidence}%`}
                {segment.startEvidence ? ` · start: ${segment.startEvidence}` : ""}
                {segment.endEvidence ? ` · end: ${segment.endEvidence}` : ""}
              </Text>
            ) : null}
            {segment.transcript ? (
              <Text
                as="pre"
                className="bg-static-800 max-h-48 overflow-auto rounded-sm p-3 whitespace-pre-wrap"
                textRole="mono"
              >
                {segment.transcript}
              </Text>
            ) : null}
          </div>
        </details>
      ) : null}
    </Stack>
  );
};

export { SplitReviewEditor };
