import { formatClipDuration } from "@loomarr/core/format";
import {
  Badge,
  Card,
  Icon,
  icons,
  PressableRow,
  Stack,
  StatusLabel,
  Text,
  VisuallyHidden,
} from "@loomarr/design-system";
import { ServedImage } from "@/components/loomarr/served-image";
import { languageName } from "@/lib/languages";
import { AUDIENCE_LABEL, type DraftSegment, isValid, spanMs } from "../split-review-draft";

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
      <Card
        overflow="hidden"
        {...(selected ? { style: { boxShadow: "0 0 0 1px var(--color-static-100)" } } : {})}
      >
        <PressableRow
          aria-current={selected ? "true" : undefined}
          onPress={(event) => onOpen(event.currentTarget as unknown as HTMLButtonElement)}
        >
          {/* The name comes from the content, so it holds every visible word (WCAG 2.5.3). */}
          <VisuallyHidden as="span">Open clip</VisuallyHidden>{" "}
          <Stack
            backgroundColor="surfaceRaised"
            borderColor="borderDecorative"
            borderRadius={8}
            borderWidth={1}
            flexShrink={0}
            height={56}
            overflow="hidden"
            position="relative"
            render="span"
            width={96}
          >
            {segment.artwork ? (
              <ServedImage image={segment.artwork} alt="" sizes="96px" />
            ) : (
              <Stack
                align="center"
                direction="row"
                height="100%"
                justify="center"
                paddingHorizontal={8}
                render="span"
                width="100%"
              >
                <Text as="span" textAlign="center" textRole="hint" tone="secondary">
                  Preview unavailable
                </Text>
              </Stack>
            )}
          </Stack>
          {/* A button centres its text; the old row said text-left. */}
          <Stack
            flexBasis="0%"
            flexGrow={1}
            flexShrink={1}
            gap="1"
            minWidth={0}
            render="span"
            style={{ textAlign: "left" }}
          >
            <Stack align="center" direction="row" gap="inline" render="span">
              <Text as="span" fontVariant={["tabular-nums"]} textRole="mono">
                #{n}
              </Text>
              <Text as="span" numberOfLines={1} textRole="cardLabel" tone="primary">
                {segment.name || "Unnamed clip"}
              </Text>
            </Stack>
            <Text
              as="span"
              fontVariant={["tabular-nums"]}
              textRole="mono"
              tone={valid ? "secondary" : "danger"}
            >
              {`${segment.startText}–${segment.endText} · ${span !== undefined && span > 0 ? formatClipDuration(span) : "invalid span"}`}
            </Text>
            <Stack align="center" direction="row" gap="1.5" marginTop={2} render="span" wrap>
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
          </Stack>
          <Icon decorative glyph={icons.rowExpand} size="compact" tone="muted" />
        </PressableRow>
      </Card>
    </li>
  );
};

export { SegmentSummaryRow };
