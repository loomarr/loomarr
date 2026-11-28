import { pluralize } from "@loomarr/core/format";
import {
  Action,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  Stack,
  Text,
} from "@loomarr/design-system";
import { useRef, useState } from "react";
import { SegmentFilmstrip } from "../segment-filmstrip";
import { SegmentInspector } from "./segment-inspector";
import { SegmentSummaryRow } from "./segment-summary-row";
import {
  type DraftSegment,
  isValid,
  languageNeedsRecheck,
  nameNowOwnedByOperator,
  resolveMs,
  toDraft,
  toWire,
} from "./split-review-draft";
import type { SplitReviewEditorProps } from "./split-review-editor.type";

// SplitReviewEditor — the §10 V34 review gate. Detection quality is a property of the
// SOURCE (measured 69–100%), so nothing enters the catalog unconfirmed: this is where the
// operator reads each proposed segment, fixes names and cut points, drops the junk, merges
// what detection over-split, and answers the era questions the grounding validator asked.
//
// The draft holds start/end as TEXT (mm:ss) so an in-progress edit can't corrupt a number;
// parsing happens at confirm time, and an unparseable or inverted span disables Confirm —
// a wrong cut is exactly what this gate exists to catch.

const SplitReviewEditor = ({
  proposal,
  minClipDurationMs,
  confirming,
  onConfirm,
  onBack,
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
      dedupUnchecked: a.dedupUnchecked || b.dedupUnchecked || undefined,
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
    <Stack gap="4" maxWidth="100%" style={{ overflowX: "hidden" }} width="100%">
      {/* The reel stays stable while the selected clip is inspected in the side panel. */}
      <SegmentFilmstrip
        segments={stripSegments}
        {...(selectedKey ? { activeKey: selectedKey } : {})}
        onSelect={openSegment}
      />

      <Stack aria-label="Clips in this recording" gap="inline" render="ol" style={{ listStyle: "none" }}>
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
        borderColor="borderDecorative"
        borderTopWidth={1}
        direction="row"
        gap="control"
        justify="between"
        paddingTop={16}
      >
        <Action onPress={onBack} variant="outline">
          Back
        </Action>
        <Stack align="center" direction="row" gap="control">
          <Text as="span" fontVariant={["tabular-nums"]} textRole="mono">
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
            <Stack gap="5" padding={24}>
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
                <Text as="span" fontVariant={["tabular-nums"]} textRole="mono">
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
    </Stack>
  );
};

export { SplitReviewEditor };
