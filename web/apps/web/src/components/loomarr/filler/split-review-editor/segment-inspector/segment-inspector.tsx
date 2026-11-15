import { formatClipDuration } from "@loomarr/core/format";
import { Action, Badge, Field, Grid, Icon, icons, Stack, Text, Tooltip } from "@loomarr/design-system";
import { languageName } from "@/lib/languages";
import { SegmentPreview } from "../../segment-preview";
import {
  AUDIENCE_LABEL,
  type DraftSegment,
  isValid,
  languageNeedsRecheck,
  nameDetail,
  nameNowOwnedByOperator,
  resolveMs,
  spanMs,
  unresolvedLanguageMessage,
} from "../split-review-draft";

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
      segment.dedupUnchecked ||
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
        <Stack flexBasis="0%" flexGrow={1} gap="2" minWidth={0}>
          <p title={valid ? undefined : "Needs mm:ss times, end after start, at least 3 seconds"}>
            <Text
              as="span"
              fontVariant={["tabular-nums"]}
              textRole="mono"
              tone={valid ? "secondary" : "danger"}
            >
              {`${segment.startText}–${segment.endText} · ${span !== undefined && span > 0 ? formatClipDuration(span) : "invalid span"}`}
            </Text>
          </p>
          <Stack align="center" direction="row" gap="1.5" wrap>
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
        </Stack>
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
        <Stack
          className="bg-caution-tint-15 rounded-sm"
          paddingHorizontal={8}
          paddingVertical={6}
          render="p"
          role="status"
        >
          <Text as="span" textRole="compact" tone="warning">
            {`Take a closer look: ${segment.holdReason}.`}
          </Text>
        </Stack>
      ) : null}
      {segment.unsplittable ? (
        <Stack className="bg-onair-tint-15 rounded-sm" paddingHorizontal={8} paddingVertical={6}>
          <Text as="p" textRole="compact" tone="danger">
            Loomarr may have missed a cut here. Preview it, then adjust the timing, join it with the next
            clip, or remove it.
          </Text>
        </Stack>
      ) : null}
      {span !== undefined && minClipDurationMs !== undefined && span < minClipDurationMs ? (
        <Stack
          className="bg-onair-tint-15 rounded-sm"
          paddingHorizontal={8}
          paddingVertical={6}
          render="p"
          role="status"
        >
          <Text as="span" textRole="compact" tone="danger">
            This clip is shorter than your {formatClipDuration(minClipDurationMs)} minimum. Make it longer,
            join it with the next clip, or remove it.
          </Text>
        </Stack>
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
        <Text as="summary" className="cursor-pointer list-none" textRole="cardLabel" tone="primary">
          <Stack align="center" direction="row" gap="2" render="span">
            <Icon decorative glyph={icons.edit} size="compact" tone="inherit" /> Rename or adjust timing
          </Stack>
        </Text>
        <Grid className="mt-3" columns={{ base: 1, sm: ["minmax(12rem,1fr)", "7rem", "7rem"] }} gap="3">
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
        </Grid>
      </details>

      {hasDetails ? (
        <details className="px-1">
          <Text as="summary" className="cursor-pointer" textRole="compact" tone="secondary">
            Details
          </Text>
          <Stack gap="2" marginTop={8}>
            {namingMessage ? (
              <Text as="p" textRole="compact" tone="secondary">
                {namingMessage}
              </Text>
            ) : null}
            {!segment.language && languageMessage ? (
              <p title={segment.languageNote}>
                <Text as="span" textRole="compact" tone="secondary">
                  {languageMessage}
                </Text>
              </p>
            ) : null}
            {segment.dupOf ? (
              <Text as="p" textRole="compact" tone="secondary">
                {`Looks like a clip already in your library: ${segment.dupOf}`}
              </Text>
            ) : null}
            {segment.dedupUnchecked && !segment.dupOf ? (
              <Text as="p" textRole="compact" tone="secondary">
                Couldn’t check this clip against your library, so it may already be there.
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
          </Stack>
        </details>
      ) : null}
    </Stack>
  );
};

export { SegmentInspector };
