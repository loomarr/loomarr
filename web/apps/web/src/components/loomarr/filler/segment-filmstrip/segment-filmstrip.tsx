import { formatClipDuration, formatMmSs } from "@loomarr/core/format";
import { useId } from "react";
import { Image } from "@/components/ui/image";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { languageName } from "@/lib/languages";
import { cn } from "@/lib/utils";
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
    <section className={cn("rounded-lg border border-border bg-muted/15 p-3", className)}>
      <div className="mb-3 flex flex-wrap items-end justify-between gap-2">
        <div>
          <h2 className="font-medium text-sm">Clips Loomarr found</h2>
          <p className="text-muted-foreground text-xs">Choose a clip to preview it and make changes.</p>
        </div>
        <span className="font-mono text-muted-foreground text-xs tabular-nums">
          {segments.length} clips · {formatMmSs(reelEnd)}
        </span>
      </div>

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
                  className="rounded-md border border-onair-300/70 border-dashed bg-onair-tint-15"
                  aria-label={label}
                  title={label}
                />
              );
            }

            const name = item.name || "Unnamed clip";
            const label = `${formatMmSs(item.startMs)} · ${name}`;
            const tooltipId = `${tooltipIdPrefix}-clip-${itemIndex}`;
            const tags = (item.tags ?? []).slice(0, 3);
            return (
              <li key={item.key} className="min-h-0 min-w-0">
                <Tooltip>
                  <TooltipTrigger
                    render={
                      <button
                        type="button"
                        aria-label={label}
                        aria-describedby={tooltipId}
                        aria-current={activeKey === item.key ? "true" : undefined}
                        onClick={() => onSelect?.(item.key)}
                        className={cn(
                          "group relative size-full overflow-hidden rounded-md border border-border bg-static-900 text-left",
                          "focus-visible:outline focus-visible:outline-2 focus-visible:outline-signal focus-visible:outline-offset-1",
                          activeKey === item.key && "ring-2 ring-signal",
                          item.unsplittable && "border-caution/70",
                        )}
                      />
                    }
                  >
                    {item.artwork ? (
                      <Image image={item.artwork} alt="" sizes="240px" className="size-full object-cover" />
                    ) : (
                      <span className="flex size-full items-center justify-center px-2 text-center text-[10px] text-muted-foreground">
                        Preview unavailable
                      </span>
                    )}
                    <span className="absolute inset-x-0 bottom-0 bg-gradient-to-t from-static-950 via-static-950/85 to-transparent px-2 pt-5 pb-1.5">
                      <span className="block truncate font-medium text-[11px] text-static-50">{name}</span>
                      <span className="font-mono text-[9px] text-static-300 tabular-nums">
                        {formatClipDuration(item.durationMs)}
                      </span>
                    </span>
                  </TooltipTrigger>
                  <TooltipContent id={tooltipId} className="w-64 p-3" side="bottom" align="start">
                    <p className="truncate font-medium text-sm">{name}</p>
                    <p className="mt-0.5 font-mono text-muted-foreground text-xs tabular-nums">
                      {formatMmSs(item.startMs)}–{formatMmSs(item.endMs)} ·{" "}
                      {formatClipDuration(item.durationMs)}
                    </p>
                    {tags.length > 0 || item.language ? (
                      <p className="mt-2 text-muted-foreground">
                        {[...tags, item.language ? languageName(item.language) : ""]
                          .filter(Boolean)
                          .join(" · ")}
                      </p>
                    ) : null}
                    {item.attention ? <p className="mt-2 text-caution">{item.attention}</p> : null}
                    <p className="mt-2 text-static-300">Click to play this exact clip.</p>
                  </TooltipContent>
                </Tooltip>
              </li>
            );
          })}
        </ul>
      </section>
    </section>
  );
};

export { SegmentFilmstrip };
