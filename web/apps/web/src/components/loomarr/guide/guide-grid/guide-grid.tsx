import type { GuideAiring } from "@loomarr/api/models/guideAiring";
import type { GuideAiringKind } from "@loomarr/api/models/guideAiringKind";
import type { GuideChannelTimeline } from "@loomarr/api/models/guideChannelTimeline";
import { guideTimeFormatter } from "@loomarr/core/guide";
import { useVirtualizer } from "@tanstack/react-virtual";
import { useLayoutEffect, useRef, useState } from "react";
import { Caption } from "@/components/ui/caption";
import { StatusDot } from "@/components/ui/status-dot";
import { cn } from "@/lib/utils";
import { ChannelIdent } from "../../channels";
import type { GuideGridProps } from "./guide-grid.type";

// GuideGrid — the cross-channel schedule (§12, built to the v2 mock): a fixed channel rail,
// then one time area per row where every block's width IS its duration.
//
// THE LAYOUT RULE. Ticks and blocks are percentages of the SAME time-area container, so
// ruler/block misalignment is structurally impossible — an earlier version positioned both in
// absolute pixels with the rail width added at one call site and not the other, and the "9:00"
// label ended up pointing at whatever was on at 8:12. Percentages survive zoom for free: the
// container's pixel width changes, the percentages inside it do not.
//
// ZOOM IS TIME, NOT CHROME. Zoom gives the time area a pixel width of `viewport × zoom` and
// lets the grid scroll horizontally, so zooming in genuinely magnifies the schedule — a 4-minute
// break that was an unreadable smear becomes a labelled block. It deliberately does NOT scale
// the rail, the row height or the type: an earlier version multiplied every font size by `zoom`,
// which meant zooming out to 75% rendered 9px titles. Chrome stays at one readable size and
// zoom changes only how many pixels an hour occupies. The window itself (2h/4h/…) is a separate
// control — that changes what you REQUEST, zoom changes how closely you look at it.
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

// Fixed chrome (see the ZOOM IS TIME note): the rail and row heights zoom no longer touches.
const RAIL_W = 220;
const ROW_H = 56;
// The time axis never collapses below this, so a very narrow viewport still shows a schedule
// rather than a sliver.
const MIN_TIME_W = 480;
// Stand-in viewport width before the real one is measured (and in jsdom, which reports 0×0).
const FALLBACK_VIEWPORT_W = 1280;
// How many viewports wide the time axis is at zoom 1. 1.6 is enough that the scrollbar is
// always present and there is real room to move forward and back, without making the default
// view so dense that the window you asked for is mostly off screen.
const OVERSCAN_FACTOR = 1.6;

// tickStep — how many minutes between ruler ticks, given the window and the zoom.
//
// Zoom buys resolution: the base step suits the window (half-hours across an evening, hours
// across a day), and each doubling of zoom halves it, down to a 5-minute floor. Without this
// the ruler stayed hourly while blocks doubled in width, so a magnified guide had all the
// detail and none of the reference points to read it against.
const tickStep = (spanMinutes: number, zoom: number): number => {
  const base = spanMinutes > LONG_WINDOW_MINUTES ? TICK_MINUTES_LONG : TICK_MINUTES_SHORT;
  const steps = [5, 10, 15, 30, 60];
  const target = base / Math.max(1, zoom);
  // The coarsest step that is still no finer than the target, so ticks never crowd.
  return steps.find((s) => s >= target) ?? base;
};

// Below this share of the row a block cannot show even a truncated word, so it renders as a
// bare colour band. A FLOOR rather than a filter: dropping short blocks would leave holes that
// every later block slides into, and the schedule would stop matching the clock.
const MIN_BLOCK_PCT = 0.25;

// The legibility thresholds are PIXELS, not percentages — and that is the fix for "you can't
// read any of the titles".
//
// They used to be shares of the window (6%), which was correct only while the time area was
// always exactly one viewport wide. Now that zoom changes the time area's pixel width, a
// percentage no longer describes a width you can read: 6% of a 2× zoomed grid is twice the
// pixels of 6% at 1×, so labels stayed hidden on blocks that had plenty of room. Measuring the
// real rendered width in px means "is there space for this text" is answered by the same
// number at every zoom — which is exactly the question being asked.
//
// 74px was MEASURED: below it "The Simpsons · Bart…" degrades to a couple of glyphs and an
// ellipsis, which reads as a rendering glitch rather than as a name.
const LABEL_MIN_PX = 74;
const CLIP_NAME_MIN_PX = 54;
// A block needs roughly this much width before the time range is worth showing at all.
const META_MIN_PX = 132;
// …and this much BLOCK HEIGHT, which is the constraint that actually binds. A series line + a
// title + a time range is ~42px of text in a 44px block: it technically fits, but with no slack
// the descenders sit flush against the border and read as clipped. So the time range yields on
// a block that already spends a line on its series — it is also the most redundant of the
// three, since the ruler directly above already says what time it is.
const META_MIN_BLOCK_H = 52;
// The block is inset from its row by `top-1.5 bottom-1.5` (6px each), so its usable height is
// the row height minus this. Kept as a named constant because the check above is comparing
// against the BLOCK, and reading `rowH` there without it silently compares the wrong box.
const BLOCK_INSET_Y = 12;

// The type scale. Fixed — see the ZOOM IS TIME note above; these do not multiply by zoom.
// Titles are 13px (up from an effective 12) and carry the row's strongest weight, because the
// title is the thing the whole surface exists to communicate.
const TITLE_PX = 13;
const META_PX = 10.5;
const CLIP_PX = 10;

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
  timezone,
  zoom = 1,
  nowMs,
  onInspect,
  onSelectChannel,
  renderRowMenu,
  className,
}: GuideGridProps) => {
  const spanMs = Math.max(1, toMs - fromMs);
  const spanMinutes = spanMs / MS_PER_MINUTE;
  const timeFmt = guideTimeFormatter(timezone);

  // Chrome is FIXED (see the zoom note at the top): the rail and rows are the same size at
  // every zoom level, so channel names stay readable and the same number of rows stay on
  // screen while you magnify the time axis.
  const railW = RAIL_W;
  const rowH = ROW_H;

  // The scroll viewport's own width, measured — the baseline the zoom model is built on.
  const scrollRef = useRef<HTMLDivElement>(null);
  const [viewportW, setViewportW] = useState(0);
  useLayoutEffect(() => {
    const el = scrollRef.current;
    if (!el) return;
    const measure = () => setViewportW(el.clientWidth);
    measure();
    if (typeof ResizeObserver === "undefined") return;
    const ro = new ResizeObserver(measure);
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  // Before the first measurement (and in jsdom, which reports every element as 0×0) fall back
  // to a sane width so the grid renders a usable layout rather than a zero-width smear.
  const effectiveViewportW = viewportW || FALLBACK_VIEWPORT_W;
  const availW = Math.max(MIN_TIME_W, effectiveViewportW - railW);

  // THE TIME AXIS IS ALWAYS WIDER THAN THE VIEWPORT — that is what gives the guide a horizontal
  // scrollbar, and it is deliberate rather than incidental.
  //
  // An earlier version defined zoom 1 as "the requested window fits exactly", which quietly
  // coupled two independent things: how much time you ASKED for (the 2h/4h/6h chips) and
  // whether you could scroll within it. At 100% there was nothing to scroll, so "scroll forward
  // or back in time" was only possible after zooming in — the control existed but the everyday
  // view could not use it.
  //
  // So the axis renders the requested window at a readable density and then keeps going: the
  // window is guaranteed at least OVERSCAN_FACTOR viewports wide, which is the scrollable
  // headroom. Zoom multiplies on top of that, so it still magnifies, but scrolling through time
  // never depends on having zoomed first.
  const timeW = Math.max(MIN_TIME_W, Math.round(availW * OVERSCAN_FACTOR * zoom));
  // What one percent of the time axis is worth in pixels — the bridge between the percentage
  // geometry (which keeps ticks and blocks aligned) and the px legibility thresholds.
  const pxPerPct = timeW / 100;

  // Ticks land on real clock boundaries, not on `fromMs` — a ruler reading "13:07, 13:37, …"
  // because that is when the request happened is unusable.
  //
  // Zoom earns finer ticks: at 2× there is room for a label every 15 minutes, and a ruler that
  // stayed hourly while the blocks doubled in width would waste the magnification.
  const tickMinutes = tickStep(spanMinutes, zoom);
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
  // Rows are a FIXED height (`rowH`), so `estimateSize` is exact rather than an estimate and no
  // dynamic measurement pass is needed. Zoom no longer changes rowH — the time axis is what
  // magnifies — so row virtualization is now entirely independent of the zoom level.
  //
  // `overscan` keeps a few rows mounted beyond the viewport so scrolling does not flash empty
  // bands, and — importantly here — so a row being hovered stays mounted while the detail card
  // reads its position.
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

  // ZOOM ANCHORING. Magnifying around the scroll container's origin would shove whatever you
  // were reading off screen — the higher the zoom, the further it travels — so zoom keeps one
  // instant pinned under the same screen position.
  //
  // The anchor is the NOW-LINE when it is on screen, because on the guide "now" is what you are
  // reading around; otherwise it is whatever time sits at the centre of the current view, which
  // is the same promise for a day you are planning rather than watching.
  //
  // Implemented against the PREVIOUS time width (kept in a ref) rather than a scroll fraction:
  // the fraction changes as the container resizes too, and using it would make a window resize
  // masquerade as a zoom. Comparing widths isolates the zoom.
  const prevTimeW = useRef(timeW);
  useLayoutEffect(() => {
    const el = scrollRef.current;
    const was = prevTimeW.current;
    prevTimeW.current = timeW;
    // Nothing to preserve on the first paint or when the width did not actually change.
    if (!el || was === timeW || was <= 0) return;

    // The anchor as a fraction of the time axis, and where it currently sits on screen.
    const viewW = el.clientWidth - railW;
    const anchorFrac = nowVisible && nowPct >= 0 ? nowPct / 100 : (el.scrollLeft + viewW / 2) / was;
    // Keep the anchor at the same screen offset it occupied before the zoom. For the now-line
    // that is wherever it already was; for a centred anchor it is the middle of the view.
    const screenOffset = nowVisible && nowPct >= 0 ? anchorFrac * was - el.scrollLeft : viewW / 2;

    const next = anchorFrac * timeW - screenOffset;
    el.scrollLeft = Math.max(0, Math.min(next, timeW - viewW));
    // `railW` is a module constant, not reactive — including it would be a no-op dependency.
  }, [timeW, nowPct, nowVisible]);

  // OPENING POSITION. The axis is now wider than the viewport even at 100%, so "scrollLeft 0"
  // is no longer the present — it is the left edge of a window that extends past both sides of
  // it. Landing there would open the guide on the earliest time it holds and make the operator
  // scroll to find what is on right now, which inverts the whole point of the surface.
  //
  // So the first paint scrolls the now-line into view, a third from the left: enough history
  // visible to see what just finished, most of the width spent on what is coming. Runs ONCE
  // (guarded by a ref) — re-running would yank the view back every time the now-line ticks, or
  // fight the operator the moment they scrolled somewhere deliberately.
  const didInitialScroll = useRef(false);
  useLayoutEffect(() => {
    const el = scrollRef.current;
    if (!el || didInitialScroll.current || !nowVisible || nowPct < 0) return;
    // Wait for the real measurement; positioning against the fallback width would land wrong
    // and then be locked in by the guard.
    if (!viewportW) return;
    didInitialScroll.current = true;
    const viewW = el.clientWidth - railW;
    const next = (nowPct / 100) * timeW - viewW / 3;
    el.scrollLeft = Math.max(0, Math.min(next, timeW - viewW));
  }, [viewportW, timeW, nowPct, nowVisible]);

  const virtualRows = rowVirtualizer.getVirtualItems();

  return (
    <div
      ref={scrollRef}
      className={cn("guide-scroll relative flex-1 overflow-auto", className)}
      data-testid="guide-grid"
      data-zoom={zoom}
    >
      {/* The grid's content box: the rail plus the ZOOMED time axis. Its width is explicit
          rather than `flex-1`, which is what lets the grid overflow and scroll horizontally at
          zoom > 1 — the flexed version could never overflow, so there was nothing to scroll and
          zoom could only ever change chrome. */}
      <div className="relative" style={{ width: railW + timeW }}>
        {/* Time ruler. Sticky so the axis stays readable while scrolling channels. */}
        <div className="sticky top-0 z-30 flex border-border border-b bg-background">
          {/* The rail corner is sticky on BOTH axes: it must stay put while the time axis
              scrolls under it, or the channel names slide away from their own rows. */}
          <div
            style={{ width: railW }}
            // z-20 for the same reason as the row rail below: the tick strip is a `relative`
            // sibling later in DOM order, so a lower layer would let tick labels scroll
            // visibly through this corner.
            className="sticky left-0 z-20 shrink-0 border-border border-r bg-background"
          />
          {/* overflow-hidden: the LAST tick sits near 100%, and its label is drawn to the
              right of that line — so the text runs past the container and widens the scroll
              area by roughly a label's width. Clipping it costs nothing (the final label is
              partially legible either way) and removes a horizontal scrollbar that appears on
              every window whose last tick lands near the edge. */}
          <div className="relative h-7.5 shrink-0 overflow-hidden" style={{ width: timeW }}>
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
                <Caption className="whitespace-nowrap">{timeFmt.format(new Date(t))}</Caption>
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
            // z-10 keeps the line UNDER the sticky rail (z-20): scrolled far enough forward,
            // the now-line's own position falls behind the rail, and a higher layer would draw
            // a red line and its time chip across the channel names.
            className="pointer-events-none absolute top-0 bottom-0 z-10 w-0.5 bg-onair"
            // Positioned off the REAL time width, not `100% - rail`: the content box is now
            // wider than the viewport when zoomed, so a percentage of the container would put
            // the line at the wrong instant the moment zoom left 1.
            style={{ left: railW + (nowPct / 100) * timeW }}
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
                  // Sticky + its own background: the time axis scrolls horizontally underneath,
                  // and a transparent rail would let programme blocks slide visibly beneath the
                  // channel names.
                  //
                  // ⚠ z-20, not z-10. The time area is a `relative` SIBLING that comes after
                  // this in DOM order, so it paints above anything at a lower or equal layer —
                  // a rail at z-10 let a block scrolled underneath it show its label through
                  // the rail (a stray "PM" floating beside the channel names). The rail must
                  // out-rank the sibling it is covering, not merely its own children.
                  // The whole first column lights on hover, not just the name (§9.1 V54). The cell
                  // IS the channel — ident, number, name, status — so lighting only the text would
                  // leave the row's own boundary as the one part that doesn't respond, and the
                  // name was a button that never looked like one.
                  //
                  // ⚠ The hover colour must stay OPAQUE. This cell is `sticky` and the programme
                  // blocks scroll underneath it; a translucent hover would let them show through —
                  // the same class of bug the z-20 note above exists for.
                  className="sticky left-0 z-20 flex shrink-0 items-center gap-2.25 border-border border-r bg-background pr-1 pl-3 transition-colors hover:bg-static-800"
                >
                  <ChannelIdent name={ch.name} number={ch.number} logo={ch.logo} size={30} />
                  <button
                    type="button"
                    onClick={() => onSelectChannel?.(ch.channelId)}
                    className="flex min-w-0 flex-1 cursor-pointer items-center gap-2.25 text-left"
                  >
                    <span className="shrink-0 font-mono font-semibold text-[19px] text-signal tabular-nums leading-none">
                      {ch.number}
                    </span>
                    <span className="min-w-0 flex-1">
                      <span className="block truncate font-medium text-[13px]">{ch.name}</span>
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

                <div style={{ height: rowH, width: timeW }} className="relative shrink-0 overflow-hidden">
                  {ch.airings?.map((a) => {
                    // Clip to the window: a programme already in progress reports its REAL
                    // start, which may precede `fromMs`.
                    const left = Math.max(0, pctOf(a.startMs));
                    const right = Math.min(100, pctOf(a.stopMs));
                    if (right <= 0 || left >= 100) return null;
                    const width = Math.max(MIN_BLOCK_PCT, right - left);

                    const airing = nowMs !== undefined && nowMs >= a.startMs && nowMs < a.stopMs;
                    // The block's REAL rendered width, which is what legibility depends on.
                    const blockPx = width * pxPerPct;
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
                        // The real element is the anchor: unlike a row-index estimate, its
                        // bounding box stays truthful through scrolling, zoom and virtualization.
                        onMouseEnter={(event) => onInspect?.(a, ch.channelId, event.currentTarget)}
                        onMouseLeave={() => onInspect?.(null)}
                        onFocus={(event) => onInspect?.(a, ch.channelId, event.currentTarget)}
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
                          clips.length > 0 ? "p-0.5" : "gap-px px-2 py-1",
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
                                  {share * pxPerPct > CLIP_NAME_MIN_PX && (
                                    <span
                                      style={{ fontSize: CLIP_PX }}
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
                          blockPx > LABEL_MIN_PX && (
                            // TWO LINES, series above title — the fix for "you can't read any of
                            // the titles". The old single line rendered "The Simpsons · Bart Gets
                            // an F" and truncated to "The Simpsons ·…", so every block on a
                            // marathon channel read IDENTICALLY: the series name is the part
                            // they share, and the episode is the part that tells them apart.
                            //
                            // Splitting them gives the episode its own line and its own budget,
                            // and makes the series the quiet one (dimmer, smaller) since it is
                            // already established by the row you are reading.
                            <>
                              {a.series && (
                                <span className="truncate font-mono text-[10px] text-static-400 uppercase leading-tight tracking-[0.04em]">
                                  {a.series}
                                </span>
                              )}
                              <span
                                style={{ fontSize: TITLE_PX }}
                                className={cn(
                                  "truncate font-medium leading-snug",
                                  // The title carries the row's strongest contrast: it is the
                                  // one thing this surface exists to communicate. Airing blocks
                                  // go brightest of all.
                                  airing ? "text-static-0" : "text-static-100",
                                )}
                              >
                                {a.series ? (a.title?.trim() ?? label) : label}
                              </span>
                              {blockPx >= META_MIN_PX &&
                                (!a.series || rowH - BLOCK_INSET_Y >= META_MIN_BLOCK_H) && (
                                  <span
                                    style={{ fontSize: META_PX }}
                                    className="truncate font-mono text-static-400 leading-tight"
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
