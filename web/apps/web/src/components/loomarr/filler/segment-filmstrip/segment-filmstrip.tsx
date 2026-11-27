import { formatClipDuration, formatMmSs } from "@loomarr/core/format";
import { Card, Grid, type GridTrack, MediaTile, Stack, Text, Tooltip } from "@loomarr/design-system";
import { useId } from "react";
import { ServedImage } from "@/components/loomarr/served-image";
import { languageName } from "@/lib/languages";
import type { FilmstripSegment, SegmentFilmstripProps } from "./segment-filmstrip.type";

type TimelineItem =
  | ({ kind: "segment"; durationMs: number } & FilmstripSegment)
  | { kind: "gap"; key: string; startMs: number; endMs: number; durationMs: number };

const timelineItems = (spans: Array<FilmstripSegment & { durationMs: number }>): TimelineItem[] => {
  const ordered = [...spans].sort((a, b) => a.startMs - b.startMs || a.endMs - b.endMs);
  const items: TimelineItem[] = [];
  let cursor = 0;
  for (const span of ordered) {
    if (span.startMs > cursor) {
      items.push({
        kind: "gap",
        key: `gap-${cursor}-${span.startMs}`,
        startMs: cursor,
        endMs: span.startMs,
        durationMs: span.startMs - cursor,
      });
    }
    items.push({ kind: "segment", ...span });
    cursor = Math.max(cursor, span.endMs);
  }
  return items;
};

// SegmentFilmstrip is the primary review overview: real stills in time order, with duration still
// controlling relative width. A minimum column width makes 50+ clips horizontally scannable rather
// than compressing them into untappable slivers; the reel scrolls instead of turning into a wall.
const SegmentFilmstrip = ({ segments, activeKey, onSelect, className }: SegmentFilmstripProps) => {
  const tooltipIdPrefix = useId();
  const spans = segments.map((segment) => ({
    ...segment,
    durationMs: Math.max(0, segment.endMs - segment.startMs),
  }));
  const positiveSpans = spans.filter((segment) => segment.durationMs > 0);
  const reelEnd = Math.max(0, ...positiveSpans.map((segment) => segment.endMs));
  if (positiveSpans.length === 0 || reelEnd <= 0) return null;

  const items = timelineItems(positiveSpans);
  const columns = items.map(
    (item): GridTrack =>
      `minmax(${item.kind === "segment" ? "6rem" : "1.5rem"}, ${Math.max(1, item.durationMs)}fr)`,
  );

  return (
    <Card
      className={className}
      gap={12}
      maxWidth="100%"
      minWidth={0}
      overflow="hidden"
      padding={12}
      render="section"
    >
      <Stack align="end" direction="row" gap="inline" justify="between" wrap>
        <div>
          <Text as="h2" textRole="cardLabel" tone="primary">
            Clips Loomarr found
          </Text>
          <Text as="p" textRole="hint" tone="secondary">
            Choose a clip to preview it and make changes.
          </Text>
        </div>
        <Text as="span" fontVariant={["tabular-nums"]} textRole="mono">
          {segments.length} clips · {formatMmSs(reelEnd)}
        </Text>
      </Stack>

      <Stack
        aria-label="Detected clip timeline"
        paddingBottom={8}
        render="section"
        style={{ overflowX: "auto" }}
      >
        <Grid
          aria-label="Detected clips, in order"
          columns={columns}
          gap="1"
          height={96}
          minWidth={Math.max(items.length * 96, 480)}
          render="ul"
          style={{ gridTemplateRows: "minmax(0, 1fr)" }}
        >
          {items.map((item, itemIndex) => {
            if (item.kind === "gap") {
              const label = `${formatMmSs(item.startMs)}–${formatMmSs(item.endMs)} unassigned`;
              return (
                <li
                  key={item.key}
                  style={{
                    backgroundColor: "var(--color-onair-tint-15)",
                    border: "1px dashed color-mix(in srgb, var(--color-onair-300) 70%, transparent)",
                    borderRadius: 8,
                  }}
                  aria-label={label}
                  title={label}
                />
              );
            }

            const name = item.name || "Unnamed clip";
            const tooltipId = `${tooltipIdPrefix}-clip-${itemIndex}`;
            const tags = (item.tags ?? []).slice(0, 3);
            return (
              <Stack key={item.key} minHeight={0} minWidth={0} render="li">
                <Tooltip
                  id={tooltipId}
                  side="bottom"
                  align="start"
                  width={256}
                  content={
                    <Stack gap="2">
                      <Stack gap="0.5">
                        <Text as="p" numberOfLines={1} textRole="cardLabel" tone="primary">
                          {name}
                        </Text>
                        <Text as="p" fontVariant={["tabular-nums"]} textRole="mono" tone="secondary">
                          {formatMmSs(item.startMs)}–{formatMmSs(item.endMs)} ·{" "}
                          {formatClipDuration(item.durationMs)}
                        </Text>
                      </Stack>
                      {tags.length > 0 || item.language ? (
                        <Text as="p" textRole="hint" tone="secondary">
                          {[...tags, item.language ? languageName(item.language) : ""]
                            .filter(Boolean)
                            .join(" · ")}
                        </Text>
                      ) : null}
                      {item.attention ? (
                        <Text as="p" textRole="hint" tone="warning">
                          {item.attention}
                        </Text>
                      ) : null}
                      <Text as="p" textRole="hint" tone="secondary">
                        Click to play this exact clip.
                      </Text>
                    </Stack>
                  }
                >
                  <MediaTile
                    aria-current={activeKey === item.key ? "true" : undefined}
                    aria-describedby={tooltipId}
                    // The current block is neutral (#1909); the amber outline is focus. The reel
                    // scrolls, so its ring is drawn inside the tile rather than clipped outside it.
                    current={activeKey === item.key}
                    focusInset
                    height="100%"
                    onPress={(event) =>
                      onSelect?.(item.key, event.currentTarget as unknown as HTMLButtonElement)
                    }
                    radius="medium"
                    tone={item.unsplittable ? "warning" : "neutral"}
                    width="100%"
                  >
                    {/* The name comes from the content, so it holds every visible word (WCAG 2.5.3);
                        the start time is the part the tile shows only to assistive tech. */}
                    <span className="sr-only">{formatMmSs(item.startMs)} ·</span>{" "}
                    {item.artwork ? (
                      <ServedImage image={item.artwork} alt="" sizes="240px" />
                    ) : (
                      <Stack
                        align="center"
                        direction="row"
                        height="100%"
                        justify="center"
                        paddingHorizontal={8}
                        width="100%"
                        render="span"
                      >
                        <Text as="span" textAlign="center" textRole="guideCell" tone="secondary">
                          Preview unavailable
                        </Text>
                      </Stack>
                    )}
                    <Stack
                      bottom={0}
                      left={0}
                      paddingBottom={6}
                      paddingHorizontal={8}
                      paddingTop={20}
                      position="absolute"
                      render="span"
                      right={0}
                      style={{
                        backgroundImage:
                          "linear-gradient(to top, var(--color-static-950), color-mix(in srgb, var(--color-static-950) 85%, transparent), transparent)",
                      }}
                    >
                      <Text as="span" numberOfLines={1} textRole="label" tone="primary">
                        {name}
                      </Text>
                      <Text as="span" fontVariant={["tabular-nums"]} textRole="microBadge">
                        {formatClipDuration(item.durationMs)}
                      </Text>
                    </Stack>
                  </MediaTile>
                </Tooltip>
              </Stack>
            );
          })}
        </Grid>
      </Stack>
    </Card>
  );
};

export { SegmentFilmstrip };
