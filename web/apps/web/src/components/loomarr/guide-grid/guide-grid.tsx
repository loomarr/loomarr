import type { GuideAiring, GuideAiringKind } from "@loomarr/api";
import { cn } from "@/lib";
import type { GuideGridProps } from "./guide-grid.type";

// GuideGrid — the cross-channel schedule (§12): one row per channel, time as the shared
// horizontal axis, every block's width IS its duration.
//
// The layout rule that makes it a grid rather than N independent strips: each block is
// positioned ABSOLUTELY against one origin (`fromMs`) at one scale (`pxPerMinute`). Sizing
// blocks as a percentage of their own row — the way PodTimeline does, correctly, for a
// self-contained pod — would let two rows disagree about where 9pm is, and a guide whose
// columns do not line up is not a guide.
//
// KIND IS NOT DECORATION. A programme, a commercial break and a still-acquiring slot are
// three different facts, and the API distinguishes them precisely so this surface can. The
// styling below is the visual half of that: a break is quieter than a programme (it is not
// what anyone tuned in for) but never invisible, because an unexplained gap in a schedule
// reads as a bug rather than as advertising.
const KIND_STYLE: Record<GuideAiringKind, string> = {
  program: "bg-tune-tint-30 border-tune-tint-30 text-static-100",
  filler: "bg-suggest-tint-30 border-suggest-tint-30 text-static-200",
  // Hatched + dashed, never solid: a pending block's times are an ESTIMATE (the API marks
  // it `nominal`), so it must not read as a scheduled programme. The stripes are the cue
  // that this slot is held, not promised.
  pending:
    "border-dashed border-signal-tint-30 text-static-300 bg-[repeating-linear-gradient(45deg,transparent,transparent_4px,rgb(255_255_255/0.06)_4px,rgb(255_255_255/0.06)_8px)]",
  flex: "bg-static-700 border-static-700 text-static-400",
};

// What each kind is called when it has no title of its own. A block still needs a label:
// an unlabelled rectangle is the "unexplained gap" problem in a different costume.
const KIND_FALLBACK_LABEL: Record<GuideAiringKind, string> = {
  program: "Programme",
  filler: "Commercials",
  pending: "Coming soon",
  flex: "Filler",
};

const MS_PER_MINUTE = 60_000;
// The sticky channel-label gutter's width. Blocks are offset by it, so it must be ONE
// number rather than a class and a matching magic constant — the two would drift and every
// block would sit a few pixels off its true time, which is invisible until it is wrong.
const LABEL_GUTTER_PX = 144;
// Below this width a block cannot show even a truncated word, so it renders as a bare
// colour band. Kept as a floor rather than hiding short blocks: dropping them would leave
// holes and shift nothing else, but the schedule would stop matching the clock.
const MIN_BLOCK_PX = 3;
// The time ruler's tick spacing. 30 minutes is what a TV guide has always used, and it
// divides the common programme lengths (22/30/45/60) legibly.
const TICK_MINUTES = 30;

// The label a block shows. An episode carries BOTH its series and its own name — "The
// Simpsons · Bart the Mother" — because either alone is a worse answer: the series name
// repeated down a row says nothing about what is on, and the episode name alone hides which
// show it belongs to.
const blockLabel = (a: GuideAiring): string => {
  const title = a.title?.trim();
  if (!title) return KIND_FALLBACK_LABEL[a.kind];
  return a.series ? `${a.series} · ${title}` : title;
};

// The tooltip/aria text: the label plus its real clock times, and — for a nominal block —
// the fact that those times are an estimate. That caveat belongs in the accessible text and
// not only in the styling, or a screen-reader user is told a placeholder is a schedule.
const blockDescription = (a: GuideAiring, fmt: Intl.DateTimeFormat): string => {
  const when = `${fmt.format(new Date(a.startMs))}–${fmt.format(new Date(a.stopMs))}`;
  const estimate = a.nominal ? " (estimated — still acquiring)" : "";
  return `${blockLabel(a)} · ${when}${estimate}`;
};

const GuideGrid = ({
  channels,
  fromMs,
  toMs,
  pxPerMinute = 4,
  nowMs,
  onSelectAiring,
  className,
}: GuideGridProps) => {
  const spanMinutes = Math.max(0, (toMs - fromMs) / MS_PER_MINUTE);
  // The scrollable width is the gutter PLUS the time axis: the labels are a column of the
  // grid, not an overlay, so omitting them here would cut the last minutes off the scroll.
  const axisWidth = spanMinutes * pxPerMinute;
  const axisEnd = LABEL_GUTTER_PX + axisWidth;
  const totalWidth = axisEnd;
  const timeFmt = new Intl.DateTimeFormat(undefined, { hour: "numeric", minute: "2-digit" });

  // Ticks are aligned to real clock half-hours, not to `fromMs` — a ruler that starts at
  // "13:07, 13:37, …" because that is when the request happened is unreadable. Round the
  // window start UP to the next boundary and step from there.
  const firstTick = Math.ceil(fromMs / (TICK_MINUTES * MS_PER_MINUTE)) * (TICK_MINUTES * MS_PER_MINUTE);
  const ticks: number[] = [];
  for (let t = firstTick; t < toMs; t += TICK_MINUTES * MS_PER_MINUTE) ticks.push(t);

  // ONE coordinate function for everything on the axis — ruler ticks, blocks, the now-line.
  // The gutter offset lives HERE rather than at each call site: when it did not, the ticks
  // (which omitted it) sat exactly one gutter-width left of the blocks (which added it), so
  // the "9:00" label pointed at whatever was on at 8:12. Every consumer of the axis must
  // share one origin or the grid quietly lies about time.
  const xOf = (ms: number) => LABEL_GUTTER_PX + ((ms - fromMs) / MS_PER_MINUTE) * pxPerMinute;
  const nowVisible = nowMs !== undefined && nowMs >= fromMs && nowMs <= toMs;

  return (
    <div className={cn("flex flex-col overflow-x-auto", className)} data-testid="guide-grid">
      <div style={{ width: totalWidth ? `${totalWidth}px` : undefined }} className="relative min-w-full">
        {/* Time ruler */}
        <div className="sticky top-0 z-20 h-6 border-border border-b bg-background">
          {ticks.map((t) => (
            <span
              key={t}
              className="absolute top-0 border-border/60 border-l pl-1 font-mono text-[10px] text-static-400"
              style={{ left: `${xOf(t)}px` }}
            >
              {timeFmt.format(new Date(t))}
            </span>
          ))}
        </div>

        {/* The now-line spans every row, so it reads as one instant across all channels
            rather than as a per-row marker. Drawn above the blocks but non-interactive, or
            it would swallow clicks on whatever is airing right now. */}
        {nowVisible && (
          <div
            aria-hidden="true"
            data-testid="guide-now-line"
            className="pointer-events-none absolute top-0 bottom-0 z-10 w-px bg-signal-500"
            style={{ left: `${xOf(nowMs)}px` }}
          />
        )}

        <ul className="flex flex-col">
          {channels.map((ch) => (
            <li key={ch.channelId} className="relative h-14 border-border border-b last:border-b-0">
              {/* The channel label rides along on horizontal scroll (sticky-left) so a row
                  never becomes anonymous when scrolled into the future. */}
              <div
                style={{ width: `${LABEL_GUTTER_PX}px` }}
                className="sticky left-0 z-10 flex h-full shrink-0 flex-col justify-center border-border border-r bg-background px-2"
              >
                <span className="truncate font-medium text-sm">{ch.name}</span>
                <span className="font-mono text-static-400 text-xs">{ch.number}</span>
              </div>

              {ch.airings?.map((a) => {
                // Clip to the window: a programme already in progress reports its REAL
                // start (which may precede `fromMs`), and drawing from that start would
                // push the block off the left edge and misalign its right end.
                // Clamp to the axis, which now BEGINS at the gutter — so a programme that
                // started before the window sits flush against the labels rather than
                // underneath them.
                const left = Math.max(LABEL_GUTTER_PX, xOf(a.startMs));
                const right = Math.min(axisEnd, xOf(a.stopMs));
                const width = Math.max(MIN_BLOCK_PX, right - left);
                if (right <= LABEL_GUTTER_PX || left >= axisEnd) return null;
                const label = blockLabel(a);
                return (
                  <button
                    type="button"
                    // startMs is the identity: a channel airs at most one thing at an
                    // instant, and the same title can legitimately repeat in a cycle, so
                    // the title alone would not be unique.
                    key={`${ch.channelId}-${a.startMs}`}
                    onClick={() => onSelectAiring?.(ch.channelId, a.startMs)}
                    title={blockDescription(a, timeFmt)}
                    aria-label={blockDescription(a, timeFmt)}
                    style={{ left: `${left}px`, width: `${width}px` }}
                    className={cn(
                      "absolute top-1 bottom-1 overflow-hidden rounded-sm border px-1.5 text-left",
                      KIND_STYLE[a.kind],
                    )}
                  >
                    <span className="block truncate text-xs leading-tight">{label}</span>
                    {a.season && a.episode ? (
                      <span className="block truncate font-mono text-[10px] opacity-70">
                        {`S${String(a.season).padStart(2, "0")}E${String(a.episode).padStart(2, "0")}`}
                      </span>
                    ) : null}
                  </button>
                );
              })}
            </li>
          ))}
        </ul>
      </div>
    </div>
  );
};

export { GuideGrid };
