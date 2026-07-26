import type { GuideAiring, GuideAiringKind, GuideChannelTimeline } from "@loomarr/api";
import { cn } from "@/lib";
import { ChannelIdent } from "../channel-ident";
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

  return (
    <div className={cn("relative flex-1 overflow-auto", className)} data-testid="guide-grid">
      <div className="relative min-w-[880px]">
        {/* Time ruler. Sticky so the axis stays readable while scrolling channels. */}
        <div className="sticky top-0 z-30 flex border-border border-b bg-background">
          <div style={{ width: railW }} className="shrink-0 border-border border-r" />
          <div className="relative h-[30px] flex-1">
            {ticks.map((t) => (
              <div
                key={t}
                data-testid="guide-tick"
                data-tick-pct={pctOf(t).toFixed(3)}
                style={{ left: `${pctOf(t)}%` }}
                className={cn(
                  "absolute top-0 bottom-0 flex items-center border-l pl-[7px]",
                  // Hour lines brighter than half-hours: the eye needs a coarse rhythm to
                  // navigate by, and uniform lines give it none.
                  new Date(t).getMinutes() === 0 ? "border-border" : "border-border/40",
                )}
              >
                <span className="whitespace-nowrap font-mono text-[10.5px] text-static-400">
                  {timeFmt.format(new Date(t))}
                </span>
              </div>
            ))}
          </div>
        </div>

        {/* The now-line spans every row, so it reads as ONE instant across all channels.
            Non-interactive, or it would swallow clicks on whatever is airing right now. */}
        {nowVisible && (
          <div
            aria-hidden="true"
            data-testid="guide-now-line"
            className="pointer-events-none absolute top-0 bottom-0 z-20 w-px bg-signal"
            style={{ left: `calc(${railW}px + (100% - ${railW}px) * ${nowPct / 100})` }}
          />
        )}

        <ul>
          {channels.map((ch) => {
            const health = healthOf(ch);
            const chip = health ? HEALTH_CHIP[health] : null;
            const onAir = onAirOf(ch);
            return (
              <li
                key={ch.channelId}
                // A channel that is off air is dimmed rather than hidden: it is still one of the
                // operator's channels, and removing the row would read as a deletion.
                style={{ opacity: onAir === "off" ? 0.55 : 1 }}
                className="flex border-border/60 border-b last:border-b-0"
              >
                <div
                  style={{ width: railW, height: rowH }}
                  className="flex shrink-0 items-center gap-[9px] border-border border-r pr-1 pl-3"
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
                    className="flex min-w-0 flex-1 items-center gap-[9px] text-left"
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
                            "mt-0.5 block font-mono text-[10px] uppercase tracking-[0.04em]",
                            chip.className,
                          )}
                        >
                          {chip.label}
                        </span>
                      )}
                    </span>
                  </button>
                  {/* On-air dot: live pulses, reconciling is amber, off is inert. */}
                  <span
                    // role="img" so the label is actually announced. aria-label is not
                    // supported on a bare <span> (implicit role: generic), and the dot
                    // carries real status — live / reconciling / off — so dropping the
                    // label rather than giving it a role would hide that from assistive
                    // tech entirely.
                    role="img"
                    aria-label={`${ch.name} is ${onAir}`}
                    data-testid="guide-onair-dot"
                    data-onair={onAir}
                    className={cn(
                      "size-2 shrink-0 rounded-full",
                      onAir === "live" && "animate-pulse bg-onair",
                      onAir === "reconciling" && "bg-signal-400",
                      onAir === "off" && "bg-static-500",
                    )}
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
                    const described = `${label} · ${when}${a.nominal ? " (estimated — still acquiring)" : ""}`;

                    return (
                      <button
                        type="button"
                        // startMs is the identity: a channel airs one thing at an instant, and
                        // the same title can legitimately repeat within a cycle.
                        key={`${ch.channelId}-${a.startMs}`}
                        data-kind={a.kind}
                        data-airing={airing || undefined}
                        onMouseEnter={() => onInspect?.(a, ch.channelId)}
                        onMouseLeave={() => onInspect?.(null)}
                        onFocus={() => onInspect?.(a, ch.channelId)}
                        onBlur={() => onInspect?.(null)}
                        title={described}
                        aria-label={described}
                        style={{ left: `${left}%`, width: `${width}%` }}
                        className={cn(
                          "absolute top-1.5 bottom-1.5 flex flex-col justify-center overflow-hidden rounded border border-l-2 text-left",
                          clips.length > 0 ? "p-0.5" : "px-2",
                          a.kind === "filler" &&
                            "border-signal-tint-40 border-l-transparent bg-signal-tint-15",
                          // Dashed + hatched, never solid: a pending block's times are an
                          // ESTIMATE, so it must not read as a scheduled programme.
                          a.kind === "pending" &&
                            "border-tune-tint-40 border-l-tune-tint-40 border-dashed bg-tune-tint-8 text-static-400",
                          a.kind === "flex" &&
                            "border-static-700 border-l-static-700 bg-static-800 text-static-400",
                          a.kind === "program" &&
                            (airing
                              ? "border-signal-tint-40 border-l-signal bg-signal-tint-12"
                              : "border-border border-l-border bg-static-800"),
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
                                    "flex min-w-0 flex-col justify-center overflow-hidden rounded-[2px] px-[3px]",
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
