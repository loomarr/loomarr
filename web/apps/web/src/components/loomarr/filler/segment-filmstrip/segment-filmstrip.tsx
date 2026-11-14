import { formatClipDuration, formatMmSs } from "@loomarr/core/format";
import { Card, MediaTile, Stack, Text, Tooltip } from "@loomarr/design-system";
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
  const columns = items
    .map(
      (item) => `minmax(${item.kind === "segment" ? "6rem" : "1.5rem"}, ${Math.max(1, item.durationMs)}fr)`,
    )
    .join(" ");

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
        <Text as="span" className="tabular-nums" textRole="mono">
          {segments.length} clips · {formatMmSs(reelEnd)}
        </Text>
      </Stack>

      <section className="overflow-x-auto pb-2" aria-label="Detected clip timeline">
        <ul
          className="grid h-24 list-none gap-1"
          style={{
            gridTemplateColumns: columns,
            gridTemplateRows: "minmax(0, 1fr)",
            minWidth: `${Math.max(items.length * 96, 480)}px`,
          }}
          aria-label="Detected clips, in order"
        >
          {items.map((item, itemIndex) => {
            if (item.kind === "gap") {
              const label = `${formatMmSs(item.startMs)}–${formatMmSs(item.endMs)} unassigned`;
              return (
                <li
                  key={item.key}
                  className="border-onair-300/70 bg-onair-tint-15 rounded-md border border-dashed"
                  aria-label={label}
                  title={label}
                />
              );
            }

            const name = item.name || "Unnamed clip";
            const tooltipId = `${tooltipIdPrefix}-clip-${itemIndex}`;
            const tags = (item.tags ?? []).slice(0, 3);
            return (
              <li key={item.key} className="min-h-0 min-w-0">
                <Tooltip
                  id={tooltipId}
                  side="bottom"
                  align="start"
                  width={256}
                  content={
                    <Stack gap="2">
                      <Stack gap="0.5">
                        <Text as="p" className="truncate" textRole="cardLabel" tone="primary">
                          {name}
                        </Text>
                        <Text as="p" className="tabular-nums" textRole="mono" tone="secondary">
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
                        className="text-muted-foreground text-center text-[10px]"
                        direction="row"
                        height="100%"
                        justify="center"
                        paddingHorizontal={8}
                        width="100%"
                        render="span"
                      >
                        Preview unavailable
                      </Stack>
                    )}
                    <span className="from-static-950 via-static-950/85 absolute inset-x-0 bottom-0 bg-gradient-to-t to-transparent px-2 pt-5 pb-1.5">
                      <span className="text-static-50 block truncate text-[11px] font-medium">{name}</span>
                      <span className="text-static-300 font-mono text-[9px] tabular-nums">
                        {formatClipDuration(item.durationMs)}
                      </span>
                    </span>
                  </MediaTile>
                </Tooltip>
              </li>
            );
          })}
        </ul>
      </section>
    </Card>
  );
};

export { SegmentFilmstrip };
