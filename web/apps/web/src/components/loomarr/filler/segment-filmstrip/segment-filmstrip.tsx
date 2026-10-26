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
    <section
      className={cn(
        "border-border bg-muted/15 max-w-full min-w-0 overflow-hidden rounded-lg border p-3",
        className,
      )}
    >
      <div className="mb-3 flex flex-wrap items-end justify-between gap-2">
        <div>
          <h2 className="text-sm font-medium">Clips Loomarr found</h2>
          <p className="text-muted-foreground text-xs">Choose a clip to preview it and make changes.</p>
        </div>
        <span className="text-muted-foreground font-mono text-xs tabular-nums">
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
                  className="border-onair-300/70 bg-onair-tint-15 rounded-md border border-dashed"
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
                        onClick={(event) => onSelect?.(item.key, event.currentTarget)}
                        className={cn(
                          "group border-border bg-static-900 relative size-full overflow-hidden rounded-md border text-left",
                          "focus-visible:outline-signal focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-1",
                          // The current block is neutral (#1909); the amber outline above is focus.
                          activeKey === item.key && "ring-foreground ring-2",
                          item.unsplittable && "border-caution/70",
                        )}
                      />
                    }
                  >
                    {item.artwork ? (
                      <Image image={item.artwork} alt="" sizes="240px" className="size-full object-cover" />
                    ) : (
                      <span className="text-muted-foreground flex size-full items-center justify-center px-2 text-center text-[10px]">
                        Preview unavailable
                      </span>
                    )}
                    <span className="from-static-950 via-static-950/85 absolute inset-x-0 bottom-0 bg-gradient-to-t to-transparent px-2 pt-5 pb-1.5">
                      <span className="text-static-50 block truncate text-[11px] font-medium">{name}</span>
                      <span className="text-static-300 font-mono text-[9px] tabular-nums">
                        {formatClipDuration(item.durationMs)}
                      </span>
                    </span>
                  </TooltipTrigger>
                  <TooltipContent id={tooltipId} className="w-64 p-3" side="bottom" align="start">
                    <p className="truncate text-sm font-medium">{name}</p>
                    <p className="text-muted-foreground mt-0.5 font-mono text-xs tabular-nums">
                      {formatMmSs(item.startMs)}–{formatMmSs(item.endMs)} ·{" "}
                      {formatClipDuration(item.durationMs)}
                    </p>
                    {tags.length > 0 || item.language ? (
                      <p className="text-muted-foreground mt-2">
                        {[...tags, item.language ? languageName(item.language) : ""]
                          .filter(Boolean)
                          .join(" · ")}
                      </p>
                    ) : null}
                    {item.attention ? <p className="text-caution mt-2">{item.attention}</p> : null}
                    <p className="text-static-300 mt-2">Click to play this exact clip.</p>
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
