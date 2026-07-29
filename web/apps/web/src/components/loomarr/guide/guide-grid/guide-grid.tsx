import type { GuideAiring, GuideAiringKind, GuideChannelTimeline } from "@loomarr/api";
import { useVirtualizer } from "@tanstack/react-virtual";
import { useRef } from "react";
import { StatusDot } from "@/components/ui";
import { cn } from "@/lib";
import { ChannelIdent } from "../../channels";
import type { GuideGridProps } from "./guide-grid.type";

// GuideGrid — the cross-channel schedule (§12, built to the v2 mock): a fixed channel rail,
// then one flexed time area per row where every block's width IS its duration.
//
// THE LAYOUT RULE. The rail is a real flex column and the programme area is its sibling, so
// ticks and blocks are percentages of the SAME flexed container. That makes ruler/block
// misalignment structurally impossible — an earlier version positioned both in absolute pixels
// with the rail width added at one call site and not the other, and the "9:00" label ended up
// pointing at whatever was on at 8:12.
//
// KIND IS NOT DECORATION. A programme, a commercial break, a still-acquiring slot and dead-time
// padding are four different facts, and the API distinguishes them precisely so this surface
// can. A break is quieter than a programme but never invisible: an unexplained gap in a
// schedule reads as a bug rather than as advertising.

// The viewport height the row virtualizer assumes before the real one is measured. Only ever
// used where measurement is unavailable (jsdom); a browser replaces it on the first frame.
const INITIAL_VIEWPORT_H = 900;

const MS_PER_MINUTE = 60_000;
// Tick spacing follows the window: half-hours read well across an evening, hourly across a day.
const TICK_MINUTES_SHORT = 30;
const TICK_MINUTES_LONG = 60;
const LONG_WINDOW_MINUTES = 180;

// Below this share of the row a block cannot show even a truncated word, so it renders as a
// bare colour band. A FLOOR rather than a filter: dropping short blocks would leave holes that
// every later block slides into, and the schedule would stop matching the clock.
const MIN_BLOCK_PCT = 0.25;
// Under these widths a label is unreadable noise rather than information.
//
// Both are shares OF THE WINDOW, so they scale with how much time is on screen: a 4-minute
// break is legible across a 2-hour window and a smear across a day, and its label should
// vanish in the second case rather than truncate.
//
// 6% was MEASURED, not guessed. At 2.2% a 4-minute break rendered "Commercials" clipped to the
// single letter "C", which reads as a rendering glitch rather than as a name — worse than
// showing nothing, because it invites the viewer to wonder what it means. A screenshot caught
// it; no DOM assertion would have, since the text was genuinely present.
const LABEL_MIN_PCT = 6;
const CLIP_NAME_MIN_PCT = 6;
// A block needs roughly this much width before its time range fits alongside the title.
const META_MIN_MINUTES = 25;

const KIND_FALLBACK_LABEL: Record<GuideAiringKind, string> = {
  program: "Programme",
  filler: "Commercials",
  pending: "Coming soon",
  flex: "Filler",
};

// The left accent — the 2px stripe that says what KIND of block this is at a glance, even when
// the block is too narrow for a label. Applied inline (see the block's `style`) because a
// Tailwind `border-l-*` utility loses to the all-sides `border-<color>` on the same element.
// Values are the token hexes: signal (airing), tune (pending), static-700 (ordinary).
const ACCENT: Record<string, string> = {
  airing: "#FFB020",
  program: "#3A3F49",
  pending: "#4CC9E8",
  // A pod is already a solid amber wash; a stripe on top of it just muddies the edge.
  filler: "transparent",
  flex: "#3A3F49",
};

// Per-clip colours inside a pod, so a break reads as a SEQUENCE rather than one flat block —
// bumper, ad, ad, bumper is legible at a glance.
const CLIP_FILL: Record<string, string> = {
  commercial: "bg-signal-tint-40",
  bumper: "bg-tune-tint-40",
  station_id: "bg-suggest-tint-40",
  psa: "bg-signal-tint-40",
  trailer: "bg-signal-tint-30",
  interstitial: "bg-static-700",
};

// The row chip says what is WRONG or in progress. A healthy channel shows nothing — a chip
// reading "fine" on every healthy row is noise on the majority of rows, and it is the
// exceptions an operator scans for.
//
// Same vocabulary as channel-health.ts (which maps ChannelDTO.status → ChannelHealth) and the
// same colours as the card, so a channel cannot read "Drift" in one place and healthy in
// another. Duplicated as a lookup rather than imported because that module takes a full
// ChannelDTO, and a guide row deliberately carries only what a row needs.
const HEALTH_CHIP: Record<string, { label: string; className: string }> = {
  "pending-slots": { label: "Filling in", className: "text-tune" },
  drift: { label: "Drift", className: "text-signal-400" },
  paused: { label: "Paused", className: "text-static-400" },
  error: { label: "Error", className: "text-onair" },
  creating: { label: "Creating", className: "text-static-400" },
};

// healthOf mirrors channel-health.ts's channelHealth for the fields a guide row carries.
// Returns "" for a healthy channel, which renders no chip.
const healthOf = (ch: GuideChannelTimeline): string => {
  switch (ch.status) {
    case "building":
      return "creating";
    case "drifted":
      return "drift";
    case "detached":
      // Tunarr no longer has the channel Loomarr manages — the one state that needs action.
      return "error";
    case "paused":
      return "paused";
    default:
      // Live but still acquiring: on air, but not yet what was asked for.
      return ch.pendingCount > 0 ? "pending-slots" : "";
  }
};

// onAirOf mirrors channelOnAir: whether a VIEWER sees anything, which is a different question
// from health. A drifted channel still broadcasts what it has, so it reads live here and drift
// in the chip.
const onAirOf = (ch: GuideChannelTimeline): "live" | "reconciling" | "off" => {
  switch (ch.status) {
    case "live":
    case "drifted":
      return "live";
    case "building":
      return "reconciling";
    default:
      return "off";
  }
};

const blockLabel = (a: GuideAiring): string => {
  const title = a.title?.trim();
  if (!title) return KIND_FALLBACK_LABEL[a.kind];
  return a.series ? `${a.series} · ${title}` : title;
};

const GuideGrid = ({
  channels,
  fromMs,
  toMs,
  zoom = 1,
  nowMs,
  onInspect,
  onSelectChannel,
  renderRowMenu,
  className,
}: GuideGridProps) => {
  const spanMs = Math.max(1, toMs - fromMs);
  const spanMinutes = spanMs / MS_PER_MINUTE;
  const timeFmt = new Intl.DateTimeFormat(undefined, { hour: "numeric", minute: "2-digit" });

  const railW = Math.round(220 * zoom);
  const rowH = Math.round(56 * zoom);

  // Ticks land on real clock boundaries, not on `fromMs` — a ruler reading "13:07, 13:37, …"
  // because that is when the request happened is unusable.
  const tickMinutes = spanMinutes > LONG_WINDOW_MINUTES ? TICK_MINUTES_LONG : TICK_MINUTES_SHORT;
  const tickMs = tickMinutes * MS_PER_MINUTE;
  const ticks: number[] = [];
  for (let t = Math.ceil(fromMs / tickMs) * tickMs; t < toMs; t += tickMs) ticks.push(t);

  const pctOf = (ms: number) => ((ms - fromMs) / spanMs) * 100;
  const nowPct = nowMs === undefined ? -1 : pctOf(nowMs);
  const nowVisible = nowPct >= 0 && nowPct <= 100;

  // Row windowing. The DOM here grows with channels × airings-in-window, so a wide window over
  // a large lineup is thousands of absolutely-positioned nodes that all re-lay-out on a zoom
  // change. The virtualizer renders only the rows near the viewport.
  //
  // Rows are a FIXED height (`rowH`, derived from zoom), so `estimateSize` is exact rather than
  // an estimate and no dynamic measurement pass is needed. Changing zoom changes rowH, which
  // re-runs the virtualizer — that is why rowH is in the dep-ish path via estimateSize.
  //
  // `overscan` keeps a few rows mounted beyond the viewport so scrolling does not flash empty
  // bands, and — importantly here — so a row being hovered stays mounted while the detail card
  // reads its position.
  const scrollRef = useRef<HTMLDivElement>(null);
  const rowVirtualizer = useVirtualizer({
    count: channels.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => rowH,
    overscan: 6,
    initialRect: { width: 1280, height: INITIAL_VIEWPORT_H },
    // jsdom reports every element as 0×0, so the default observer would report a zero-height
    // viewport and the virtualizer would mount NO rows — not a rendering bug, but it makes
    // every unit test assert against an empty grid. This wrapper substitutes a usable height
    // ONLY when measurement comes back as zero, which never happens in a real browser, so
    // production behaviour is untouched and the tests exercise the same code path as the app.
    observeElementRect: (instance, cb) => {
      const el = instance.scrollElement;
      if (!el) return;
      const report = () => {
        const r = el.getBoundingClientRect();
        cb({ width: r.width || 1280, height: r.height || INITIAL_VIEWPORT_H });
      };
      report();
      if (typeof ResizeObserver === "undefined") return;
      const ro = new ResizeObserver(report);
      ro.observe(el);
      return () => ro.disconnect();
    },
  });
  const virtualRows = rowVirtualizer.getVirtualItems();

  return (
    <div ref={scrollRef} className={cn("relative flex-1 overflow-auto", className)} data-testid="guide-grid">
      {/* min-w scales with zoom (the mock's `gridMin: 1000 * z`) instead of being a fixed
          880px. A constant floor meant the grid demanded 880px even when zoomed out to 75%,
          so a viewport with room to spare still showed a horizontal scrollbar. Now zooming
          out genuinely makes it fit — which is what the zoom control is for. */}
      <div className="relative" style={{ minWidth: Math.round(1000 * zoom) }}>
        {/* Time ruler. Sticky so the axis stays readable while scrolling channels. */}
        <div className="sticky top-0 z-30 flex border-border border-b bg-background">
          <div style={{ width: railW }} className="shrink-0 border-border border-r" />
          {/* overflow-hidden: the LAST tick sits near 100%, and its label is drawn to the
              right of that line — so the text runs past the container and widens the scroll
              area by roughly a label's width. Clipping it costs nothing (the final label is
              partially legible either way) and removes a horizontal scrollbar that appears on
              every window whose last tick lands near the edge. */}
          <div className="relative h-7.5 flex-1 overflow-hidden">
            {ticks.map((t) => (
              <div
                key={t}
                data-testid="guide-tick"
                data-tick-pct={pctOf(t).toFixed(3)}
                style={{ left: `${pctOf(t)}%` }}
                className={cn(
                  "absolute top-0 bottom-0 flex items-center border-l pl-1.75",
                  // Hour lines brighter than half-hours: the eye needs a coarse rhythm to
                  // navigate by, and uniform lines give it none.
                  new Date(t).getMinutes() === 0 ? "border-border" : "border-border/40",
                )}
              >
                <span className="whitespace-nowrap font-mono text-2xs text-static-400">
                  {timeFmt.format(new Date(t))}
                </span>
              </div>
            ))}
          </div>
        </div>

        {/* The now-line spans every row, so it reads as ONE instant across all channels.
            Non-interactive, or it would swallow clicks on whatever is airing right now.
            `onair` (red), not `signal` (amber): amber is the brand and marks what is AIRING,
            so an amber line through amber blocks disappears into them. Red is the only accent
            reserved for live, and 2px because a hairline is lost against block borders. */}
        {nowVisible && (
          <div
            aria-hidden="true"
            data-testid="guide-now-line"
            className="pointer-events-none absolute top-0 bottom-0 z-20 w-0.5 bg-onair"
            style={{ left: `calc(${railW}px + (100% - ${railW}px) * ${nowPct / 100})` }}
          >
            {/* The time chip: a bare line says "here", not "here is 9:42". */}
            <span className="absolute top-1 -left-5 whitespace-nowrap rounded-[3px] bg-onair px-1.25 py-px font-mono text-[9.5px] text-static-950 leading-none">
              {timeFmt.format(new Date(nowMs ?? 0))}
            </span>
          </div>
        )}

        {/* The list is a SPACER of the full scroll height, with only the visible rows mounted
            inside it, absolutely positioned at their real offsets. The spacer is what keeps the
            scrollbar honest — it must be the height every row would occupy, not the height of
            what is rendered.
            ⚠ `w-full` is load-bearing. Absolutely-positioned rows no longer stretch their
            parent the way in-flow rows did, so without an explicit width the spacer collapses
            to zero and every row's `width: 100%` resolves against nothing — which brought the
            horizontal scrollbar straight back the moment rows were virtualized. */}
        <ul className="relative w-full" style={{ height: rowVirtualizer.getTotalSize() }}>
          {virtualRows.map((vr) => {
            const ch = channels[vr.index];
            if (!ch) return null;
            const rowIndex = vr.index;
            const health = healthOf(ch);
            const chip = health ? HEALTH_CHIP[health] : null;
            const onAir = onAirOf(ch);
            return (
              <li
                key={ch.channelId}
                data-testid="guide-row"
                // A channel that is off air is dimmed rather than hidden: it is still one of the
                // operator's channels, and removing the row would read as a deletion.
                style={{
                  opacity: onAir === "off" ? 0.55 : 1,
                  position: "absolute",
                  top: 0,
                  left: 0,
                  width: "100%",
                  height: vr.size,
                  transform: `translateY(${vr.start}px)`,
                }}
                className="flex border-border/60 border-b"
              >
                <div
                  style={{ width: railW, height: rowH }}
                  className="flex shrink-0 items-center gap-2.25 border-border border-r pr-1 pl-3"
                >
                  <ChannelIdent
                    name={ch.name}
                    number={ch.number}
                    logo={ch.logo}
                    size={Math.round(30 * zoom)}
                  />
                  <button
                    type="button"
                    onClick={() => onSelectChannel?.(ch.channelId)}
                    className="flex min-w-0 flex-1 items-center gap-2.25 text-left"
                  >
                    <span
                      style={{ fontSize: 19 * zoom }}
                      className="shrink-0 font-mono font-semibold text-signal tabular-nums leading-none"
                    >
                      {ch.number}
                    </span>
                    <span className="min-w-0 flex-1">
                      <span style={{ fontSize: 13 * zoom }} className="block truncate font-medium">
                        {ch.name}
                      </span>
                      {/* The chip states what is WRONG (or in progress); a healthy channel
                        shows nothing, because a row saying "fine" on every healthy channel
                        is noise on the majority of rows. */}
                      {chip && (
                        <span
                          className={cn(
                            "mt-0.5 block font-mono text-2xs uppercase tracking-[0.04em]",
                            chip.className,
                          )}
                        >
                          {chip.label}
                        </span>
                      )}
                    </span>
                  </button>
                  {/* On-air dot — the StatusDot primitive, which owns the tone→colour map,
                      the reserved-for-live pulse, and the role="img" labelling. */}
                  <StatusDot
                    tone={onAir === "live" ? "live" : onAir === "reconciling" ? "pending" : "off"}
                    label={`${ch.name} is ${onAir}`}
                    data-testid="guide-onair-dot"
                    data-onair={onAir}
                  />
                  {renderRowMenu?.(ch)}
                </div>

                <div style={{ height: rowH }} className="relative flex-1 overflow-hidden">
                  {ch.airings?.map((a) => {
                    // Clip to the window: a programme already in progress reports its REAL
                    // start, which may precede `fromMs`.
                    const left = Math.max(0, pctOf(a.startMs));
                    const right = Math.min(100, pctOf(a.stopMs));
                    if (right <= 0 || left >= 100) return null;
                    const width = Math.max(MIN_BLOCK_PCT, right - left);

                    const airing = nowMs !== undefined && nowMs >= a.startMs && nowMs < a.stopMs;
                    const minutes = (a.stopMs - a.startMs) / MS_PER_MINUTE;
                    const clips = a.pod?.entries ?? [];
                    const podTotal = clips.reduce((n, c) => n + (c.durationMs || 0), 0) || 1;
                    const label = blockLabel(a);
                    const when = `${timeFmt.format(new Date(a.startMs))}–${timeFmt.format(new Date(a.stopMs))}`;
                    // The caveat rides in the accessible name, not only in the styling — a
                    // screen-reader user must not be told a placeholder is a schedule.
                    const described = `${label} · ${when}${a.nominal ? " (estimated, still acquiring)" : ""}`;

                    return (
                      <button
                        type="button"
                        // startMs is the identity: a channel airs one thing at an instant, and
                        // the same title can legitimately repeat within a cycle.
                        key={`${ch.channelId}-${a.startMs}`}
                        data-kind={a.kind}
                        data-airing={airing || undefined}
                        // The anchor is the block's own left edge + its row, so the caller can
                        // open the detail card beside what is being inspected instead of in a
                        // fixed corner — and flip it when the block is near an edge.
                        onMouseEnter={() => onInspect?.(a, ch.channelId, { leftPct: left, rowIndex })}
                        onMouseLeave={() => onInspect?.(null)}
                        onFocus={() => onInspect?.(a, ch.channelId, { leftPct: left, rowIndex })}
                        onBlur={() => onInspect?.(null)}
                        // Clicking a block opens ITS CHANNEL — the same destination the rail's
                        // channel button goes to. There is no per-programme page to open (a
                        // block is a slot in a lineup, not an object with a route), and the
                        // channel is where everything you would want to change about it lives.
                        // Hovering already tells you about the programme; clicking acts on it.
                        onClick={() => onSelectChannel?.(ch.channelId)}
                        title={described}
                        aria-label={described}
                        // ⚠ The left accent is an INLINE style, not a `border-l-*` utility.
                        // Tailwind emits `border-<color>` (all sides) and `border-l-<color>`
                        // in a fixed order decided at build time, so a class-based accent
                        // loses to the all-sides colour no matter how the strings are ordered
                        // in `cn()` — which is exactly how the airing accent silently rendered
                        // as #2A2E37 while looking correct in the source.
                        style={{
                          left: `${left}%`,
                          width: `${width}%`,
                          borderLeftColor: ACCENT[a.kind === "program" && airing ? "airing" : a.kind],
                        }}
                        className={cn(
                          "absolute top-1.5 bottom-1.5 flex cursor-pointer flex-col justify-center overflow-hidden rounded border border-l-2 text-left",
                          // Hover/focus: the border brightens and a 1px ring lifts the block off
                          // the row. Blocks sit edge to edge, so a fill change alone is ambiguous
                          // about WHICH block is under the pointer — the outline is what makes
                          // the boundary unmistakable. Focus-visible carries the same treatment,
                          // so keyboard inspection reads identically to hover.
                          "transition-[box-shadow,border-color] duration-100 ease-out",
                          "hover:border-static-400 focus-visible:border-static-400 focus-visible:outline-none",
                          clips.length > 0 ? "p-0.5" : "px-2",
                          a.kind === "filler" &&
                            "border-signal-tint-40 bg-signal-tint-30 hover:shadow-[0_0_0_1px_#D6409F] focus-visible:shadow-[0_0_0_1px_#D6409F]",
                          // Dashed, never solid: a pending block's times are an ESTIMATE, so it
                          // must not read as a scheduled programme.
                          a.kind === "pending" &&
                            "border-tune-tint-40 border-dashed bg-tune-tint-8 text-static-400 hover:shadow-[0_0_0_1px_#4CC9E8] focus-visible:shadow-[0_0_0_1px_#4CC9E8]",
                          a.kind === "flex" &&
                            "border-static-700 bg-static-800 text-static-400 hover:shadow-[0_0_0_1px_#8B93A3] focus-visible:shadow-[0_0_0_1px_#8B93A3]",
                          a.kind === "program" &&
                            (airing
                              ? "border-signal-tint-40 bg-signal-tint-12 hover:shadow-[0_0_0_1px_#FFB020] focus-visible:shadow-[0_0_0_1px_#FFB020]"
                              : "border-border bg-static-800 hover:shadow-[0_0_0_1px_#E7EAF0] focus-visible:shadow-[0_0_0_1px_#E7EAF0]"),
                        )}
                      >
                        {clips.length > 0 ? (
                          // A pod renders its CLIPS, proportionally — a break is a sequence
                          // (bumper → ads → bumper), and one flat rectangle hides that.
                          <div className="flex h-full items-stretch gap-px">
                            {clips.map((c, i) => {
                              const share = ((c.durationMs || 0) / podTotal) * width;
                              return (
                                <div
                                  // Position is identity inside a pod: one clip may legitimately
                                  // appear twice, and the fallback card has no path at all.
                                  // biome-ignore lint/suspicious/noArrayIndexKey: position is identity in a pod
                                  key={`${c.path ?? "card"}-${i}`}
                                  title={[c.name, c.era || null, c.quality || null]
                                    .filter(Boolean)
                                    .join(" · ")}
                                  style={{ flex: `${((c.durationMs || 0) / podTotal) * 100}` }}
                                  className={cn(
                                    "flex min-w-0 flex-col justify-center overflow-hidden rounded-xs px-0.75",
                                    CLIP_FILL[c.kind] ?? "bg-static-700",
                                  )}
                                >
                                  {share > CLIP_NAME_MIN_PCT && (
                                    <span
                                      style={{ fontSize: 10 * zoom }}
                                      className="truncate text-static-0 leading-tight"
                                    >
                                      {c.name}
                                    </span>
                                  )}
                                </div>
                              );
                            })}
                          </div>
                        ) : (
                          width > LABEL_MIN_PCT && (
                            <>
                              <span style={{ fontSize: 12 * zoom }} className="truncate leading-snug">
                                {label}
                              </span>
                              {minutes >= META_MIN_MINUTES && (
                                <span
                                  style={{ fontSize: 10 * zoom }}
                                  className="truncate font-mono text-static-400"
                                >
                                  {when}
                                </span>
                              )}
                            </>
                          )
                        )}
                      </button>
                    );
                  })}
                </div>
              </li>
            );
          })}
        </ul>
      </div>
    </div>
  );
};

export { GuideGrid };
