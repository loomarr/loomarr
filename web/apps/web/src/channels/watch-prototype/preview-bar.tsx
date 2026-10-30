// PROTOTYPE — throwaway (#2088 step 0). Two answers to "the playbar shows what's coming as
// previews", for variant B:
//
//   FilmstripBar — the bar IS a row of preview tiles (now, each break with its clips, next, …),
//                  over a thin time-true track that keeps the 15-min seekable past and the playhead.
//   HoverBar     — one time-proportional bar whose segments carry a tiny still or the break's clip
//                  ticks; pointing at a segment opens a full preview card (forced per state here).
//
// Data is the guide's: programme airings (title, series, S/E, still) and a break's `pod.entries`
// (name, kind, brand, era, duration). Hues stand in for the real thumbnails.
import { Film, Megaphone, RadioTower, Sparkles } from "lucide-react";
import type { CSSProperties } from "react";
import { airings, type BreakClip, breaks, MIN, NOW, stillHue } from "./prototype-data";
import type { WatchState } from "./prototype-state";

const clock = (ms: number) => new Date(ms).toLocaleTimeString([], { hour: "numeric", minute: "2-digit" });
const mins = (ms: number) => Math.round(ms / MIN);

// The window: 15 min of seekable past, then far enough ahead to reach the evening's movie.
const PAST = 15 * MIN;
const AHEAD = 110 * MIN;
const toPct = (ms: number) => ((ms - (NOW - PAST)) / (PAST + AHEAD)) * 100;

// A stand-in picture for a still or a clip thumbnail, built like a lit frame rather than a swatch:
// sky → bright horizon → dark sea, a light source, a foreground mass, and a lens vignette. Tokens'
// hues only — no photos, no generated art.
const Still = ({ hue, className, style }: { hue: number; className?: string; style?: CSSProperties }) => (
  <div
    aria-hidden
    className={`overflow-hidden ${className ?? ""}`}
    style={{
      background: [
        "radial-gradient(ellipse 120% 95% at 50% 45%, transparent 42%, rgb(0 0 0 / .7) 100%)",
        `radial-gradient(ellipse 48% 38% at 14% 100%, hsl(${hue} 45% 5%) 97%, transparent 100%)`,
        `radial-gradient(circle at 70% 42%, hsl(${hue + 40} 95% 82% / .95) 0 5%, hsl(${hue + 30} 90% 66% / .45) 6%, transparent 26%)`,
        `linear-gradient(180deg, hsl(${hue} 50% 13%) 0%, hsl(${hue + 20} 58% 36%) 46%, hsl(${hue + 45} 88% 62%) 57%, hsl(${hue} 42% 17%) 58%, hsl(${hue} 38% 6%) 100%)`,
      ].join(","),
      ...style,
    }}
  />
);

const kindIcon = (kind: BreakClip["kind"]) =>
  kind === "station_id" || kind === "bumper" ? (
    <RadioTower className="size-3" />
  ) : kind === "trailer" ? (
    <Film className="size-3" />
  ) : (
    <Megaphone className="size-3" />
  );
const kindLabel = (c: BreakClip) =>
  c.kind === "station_id"
    ? "Station ID"
    : c.kind === "bumper"
      ? "Bumper"
      : c.kind === "trailer"
        ? "Coming up"
        : `${c.brand} · ${c.era}`;

const programmeName = (a: (typeof airings)[number]) => a.series ?? a.title;
const programmeSub = (a: (typeof airings)[number]) =>
  a.series ? `S${a.season} E${a.episode} "${a.title}"` : `${a.year} · ${a.genres?.[0] ?? "Movie"}`;

// The time-true track shared by both bars: past window (seekable), now line, playhead, ticks.
const Track = ({ lagMs, tall }: { lagMs: number; tall?: boolean }) => (
  <div className="relative">
    <div className={`bg-static-800/80 relative overflow-hidden rounded-full ${tall ? "h-1.5" : "h-1"}`}>
      <div className="bg-signal/30 absolute inset-y-0 left-0" style={{ width: `${toPct(NOW)}%` }} />
      <div className="bg-signal absolute inset-y-0 left-0" style={{ width: `${toPct(NOW - lagMs)}%` }} />
      {airings
        .filter((a) => a.kind === "filler")
        .map((a) => (
          <div
            key={a.scheduleBlockId}
            className="bg-tune-300/70 absolute inset-y-0"
            style={{ left: `${toPct(a.startMs)}%`, width: `${toPct(a.stopMs) - toPct(a.startMs)}%` }}
          />
        ))}
    </div>
    <div
      className="bg-signal absolute top-1/2 size-3 -translate-x-1/2 -translate-y-1/2 rounded-full shadow-[0_0_8px_rgb(255_176_32/.8)]"
      style={{ left: `${toPct(NOW - lagMs)}%` }}
    />
    <div className="text-static-400 relative mt-1 h-3.5 font-mono text-[10px]">
      <span className="absolute left-0">−15m</span>
      <span
        className="text-onair-300 absolute -translate-x-1/2 font-semibold"
        style={{ left: `${toPct(NOW)}%` }}
      >
        LIVE
      </span>
      {[30, 60, 90].map((m) => (
        <span key={m} className="absolute -translate-x-1/2" style={{ left: `${toPct(NOW + m * MIN)}%` }}>
          {clock(NOW + m * MIN)}
        </span>
      ))}
    </div>
  </div>
);

// ---- FilmstripBar -----------------------------------------------------------------------------------

const ProgrammeTile = ({
  a,
  now,
  lagMs,
  touch,
}: {
  a: (typeof airings)[number];
  now: boolean;
  lagMs: number;
  touch: boolean;
}) => {
  const progress = now ? ((NOW - lagMs - a.startMs) / (a.stopMs - a.startMs)) * 100 : 0;
  return (
    <button
      type="button"
      className={`group/tile flex shrink-0 cursor-pointer flex-col gap-1.5 text-left ${now ? (touch ? "w-48" : "w-60") : touch ? "w-36" : "w-44"}`}
    >
      <div
        className={`relative aspect-video w-full overflow-hidden rounded-md ring-1 ${now ? "ring-signal" : "ring-static-0/10 group-hover/tile:ring-static-0/40"}`}
      >
        <Still hue={stillHue[a.scheduleBlockId] ?? 200} className="absolute inset-0" />
        <span
          className={`absolute top-1.5 left-1.5 rounded px-1.5 py-0.5 font-mono text-[10px] font-bold ${now ? "bg-onair-300 text-static-950" : "text-static-100 bg-black/70"}`}
        >
          {now ? "NOW" : clock(a.startMs)}
        </span>
        <span className="text-static-100 absolute right-1.5 bottom-1.5 rounded bg-black/70 px-1 font-mono text-[10px]">
          {mins(a.stopMs - a.startMs)}m
        </span>
        {now && (
          <div className="absolute inset-x-0 bottom-0 h-1 bg-black/60">
            <div className="bg-signal h-full" style={{ width: `${progress}%` }} />
          </div>
        )}
      </div>
      <div className="min-w-0">
        <p className="text-static-50 truncate text-xs font-semibold">{programmeName(a)}</p>
        <p className="text-static-400 truncate text-[11px]">{programmeSub(a)}</p>
      </div>
    </button>
  );
};

const BreakTile = ({
  a,
  airing,
  touch,
}: {
  a: (typeof airings)[number];
  airing: boolean;
  touch: boolean;
}) => {
  const clips = breaks[a.scheduleBlockId] ?? [];
  return (
    <button
      type="button"
      className={`flex shrink-0 cursor-pointer flex-col gap-1.5 text-left ${touch ? "w-28" : "w-36"}`}
    >
      <div
        className={`relative grid aspect-video w-full grid-cols-3 grid-rows-2 gap-px overflow-hidden rounded-md bg-black ring-1 ${airing ? "ring-tune-300" : "ring-tune-300/30"}`}
      >
        {clips.slice(0, 6).map((c, i) => (
          <div key={c.name} className="relative">
            <Still hue={c.hue} className="absolute inset-0" />
            {airing && i === 0 && <div className="ring-tune-300 absolute inset-0 ring-2 ring-inset" />}
          </div>
        ))}
        <span
          className={`absolute top-1.5 left-1.5 rounded px-1.5 py-0.5 font-mono text-[10px] font-bold ${airing ? "bg-tune-300 text-static-950" : "text-tune-300 bg-black/70"}`}
        >
          {airing ? "NOW" : clock(a.startMs)}
        </span>
      </div>
      <div className="min-w-0">
        <p className="text-tune-300 truncate text-xs font-semibold">
          Break · {mins(a.stopMs - a.startMs)} min
        </p>
        <p className="text-static-400 truncate text-[11px]">
          {clips.length} spots · {clips.find((c) => c.kind === "commercial")?.brand}…
        </p>
      </div>
    </button>
  );
};

const FilmstripBar = ({ state, touch }: { state: WatchState; touch: boolean }) => {
  const lagMs = state === "behind" ? 134_000 : 0;
  const inBreak = state === "commercials";
  return (
    <div className="space-y-2">
      <div className="flex [scrollbar-width:none] items-start gap-2.5 overflow-x-auto pb-1">
        {/* The 15 min the stream still holds, before this programme began: one seekable tile. */}
        <button
          type="button"
          className="border-static-0/15 text-static-300 hover:border-signal/60 hover:text-static-100 flex aspect-[9/16] w-12 shrink-0 cursor-pointer flex-col items-center justify-center gap-1 self-start rounded-md border border-dashed font-mono text-[10px] leading-tight"
          title="Rewind: the last 15 minutes are still on the stream"
        >
          <span>⟲</span>
          <span>−15m</span>
        </button>
        {airings.map((a) =>
          a.kind === "filler" ? (
            <BreakTile
              key={a.scheduleBlockId}
              a={a}
              airing={inBreak && a.scheduleBlockId === "b2"}
              touch={touch}
            />
          ) : (
            <ProgrammeTile
              key={a.scheduleBlockId}
              a={a}
              now={!inBreak && a.startMs <= NOW && NOW < a.stopMs}
              lagMs={lagMs}
              touch={touch}
            />
          ),
        )}
      </div>
      <Track lagMs={lagMs} />
    </div>
  );
};

// ---- HoverBar -------------------------------------------------------------------------------------

// The programme card: the still is the hero, the title sits ON it (over a scrim), the facts below.
const ProgrammeCard = ({ a }: { a: (typeof airings)[number] }) => (
  <div className="w-[22rem]">
    <div className="relative aspect-video">
      <Still hue={stillHue[a.scheduleBlockId] ?? 200} className="absolute inset-0" />
      <div className="from-static-950 via-static-950/25 absolute inset-0 bg-linear-to-t to-transparent" />
      <span className="bg-signal text-static-950 absolute top-2.5 left-2.5 rounded-full px-2 py-0.5 font-mono text-[10px] font-bold shadow-lg">
        IN {mins(a.startMs - NOW)} MIN
      </span>
      <div className="absolute inset-x-3.5 bottom-2.5">
        <p className="text-static-200 font-mono text-[11px]">
          {clock(a.startMs)} – {clock(a.stopMs)}
        </p>
        <p className="text-static-0 text-xl leading-tight font-semibold [text-shadow:0_2px_12px_rgb(0_0_0/.8)]">
          {programmeName(a)}
        </p>
      </div>
    </div>
    <div className="px-3.5 pt-2 pb-3.5">
      <div className="flex flex-wrap gap-1.5">
        {[
          a.series ? `S${a.season} E${a.episode} "${a.title}"` : String(a.year),
          a.rating,
          ...(a.genres ?? []),
        ]
          .filter(Boolean)
          .map((chip) => (
            <span
              key={chip}
              className="text-static-200 rounded-md bg-white/[.06] px-1.5 py-0.5 text-[11px] ring-1 ring-white/10"
            >
              {chip}
            </span>
          ))}
      </div>
      <p className="text-static-300 mt-2.5 line-clamp-2 text-xs leading-relaxed">{a.description}</p>
    </div>
  </div>
);

const BreakCard = ({ a, airingIndex }: { a: (typeof airings)[number]; airingIndex?: number }) => {
  const clips = breaks[a.scheduleBlockId] ?? [];
  return (
    <div className="w-[22rem]">
      <p className="text-tune-300 from-tune-300/15 flex items-center gap-1.5 border-b border-white/8 bg-linear-to-r to-transparent px-3.5 py-2.5 text-sm font-semibold">
        <Sparkles className="size-3.5" />
        Break · {clock(a.startMs)} · {mins(a.stopMs - a.startMs)} min
      </p>
      <ol className="space-y-1 p-2">
        {clips.map((c, i) => (
          <li
            key={c.name}
            className={`flex items-center gap-3 rounded-lg p-1.5 transition-colors ${
              i === airingIndex
                ? "bg-tune-300/12 ring-tune-300/50 shadow-[0_8px_20px_-8px_rgb(0_0_0/.9)] ring-1"
                : "hover:bg-white/[.04]"
            }`}
          >
            <Still
              hue={c.hue}
              className="aspect-video w-20 shrink-0 rounded-md shadow-md ring-1 ring-white/10"
            />
            <div className="min-w-0 flex-1">
              <p className="text-static-50 truncate text-xs">{c.name}</p>
              <p className="text-static-400 flex items-center gap-1 text-[11px]">
                {kindIcon(c.kind)}
                {kindLabel(c)}
              </p>
            </div>
            <span className="text-static-400 shrink-0 font-mono text-[10px]">
              {i === airingIndex ? "ON NOW" : `${c.durationS}s`}
            </span>
          </li>
        ))}
      </ol>
    </div>
  );
};

const HoverBar = ({ state, touch }: { state: WatchState; touch: boolean }) => {
  const lagMs = state === "behind" ? 134_000 : 0;
  // Which segment the pointer is on, forced per state so each card can be judged. Touch has no
  // pointer to hover with, and a floating card would not fit a landscape phone anyway: there a tap
  // on a segment opens the same content in the right-hand sheet, so no floating card is drawn.
  const focus = touch
    ? undefined
    : state === "commercials"
      ? "b2"
      : state === "playing"
        ? "b3"
        : state === "idle"
          ? undefined
          : "b4";
  const focused = airings.find((a) => a.scheduleBlockId === focus);
  const head = toPct(NOW - lagMs);
  const focusMid = focused ? (toPct(focused.startMs) + toPct(Math.min(focused.stopMs, NOW + AHEAD))) / 2 : 0;
  return (
    <div className="relative">
      {focused && (
        <div
          className="absolute bottom-full z-20 mb-5 -translate-x-1/2"
          style={{ left: `clamp(11rem, ${focusMid}%, calc(100% - 11rem))` }}
        >
          <div className="bg-static-950/90 overflow-hidden rounded-xl shadow-[0_28px_60px_-14px_rgb(0_0_0/.95),inset_0_1px_0_rgb(255_255_255/.08)] ring-1 ring-white/12 backdrop-blur-xl">
            {focused.kind === "filler" ? (
              <BreakCard a={focused} airingIndex={state === "commercials" ? 0 : undefined} />
            ) : (
              <ProgrammeCard a={focused} />
            )}
          </div>
        </div>
      )}
      {/* The tray: a lit glass shelf the segments stand on, so the bar has a surface of its own. */}
      <div className="rounded-xl bg-linear-to-b from-white/[.08] to-white/[.02] p-1.5 shadow-[inset_0_1px_0_rgb(255_255_255/.09),0_14px_34px_-12px_rgb(0_0_0/.9)] ring-1 ring-white/10 backdrop-blur-xl">
        <div className={`relative flex items-end gap-1 ${touch ? "h-11" : "h-16"}`}>
          {/* Connector: the open card points down at the segment it describes. */}
          {focused && (
            <div
              className="pointer-events-none absolute bottom-full z-20 h-6 w-px -translate-x-1/2 bg-linear-to-t from-white/70 to-transparent"
              style={{ left: `${focusMid}%` }}
            />
          )}
          {/* Before this programme: the rest of the seekable 15 min. */}
          <div
            className="bg-signal/12 ring-signal/25 h-full shrink-0 rounded-md ring-1"
            style={{ width: `calc(${toPct(airings[0]?.startMs ?? NOW)}% - 4px)` }}
            title="Rewind"
          />
          {airings.map((a) => {
            const w = toPct(Math.min(a.stopMs, NOW + AHEAD)) - toPct(a.startMs);
            if (w <= 0) return null;
            const on = a.scheduleBlockId === focus;
            const isNow = a.startMs <= NOW && NOW < a.stopMs;
            // Depth by light only — every segment is the same height and nothing moves. Now and the
            // pointed-at segment are fully lit; the rest of the future is dimmed, further away.
            const depth = on
              ? "h-full brightness-110 z-10"
              : isNow
                ? "h-full"
                : "h-full brightness-[.72] saturate-[.7] hover:brightness-100 hover:saturate-100";
            if (a.kind === "filler") {
              const clips = breaks[a.scheduleBlockId] ?? [];
              const total = clips.reduce((s, c) => s + c.durationS, 0);
              return (
                <div
                  key={a.scheduleBlockId}
                  // A break never drops below a hoverable width; programmes give up the difference.
                  className={`bg-tune-300/15 flex min-w-9 shrink-0 gap-[2px] rounded-md p-[2px] transition-all duration-300 ${
                    on ? "ring-tune-300 ring-2" : "ring-tune-300/35 ring-1"
                  } ${depth}`}
                  style={{ width: `calc(${w}% - 4px)` }}
                  title="Break"
                >
                  {clips.map((c) => (
                    <Still
                      key={c.name}
                      hue={c.hue}
                      className="h-full rounded-[3px]"
                      style={{ width: `${(c.durationS / total) * 100}%` }}
                    />
                  ))}
                </div>
              );
            }
            return (
              <div
                key={a.scheduleBlockId}
                className={`relative min-w-0 overflow-hidden rounded-md transition-all duration-300 ${
                  on ? "ring-2 ring-white/90" : isNow ? "ring-signal/70 ring-1" : "ring-1 ring-white/10"
                } ${depth}`}
                style={{ width: `calc(${w}% - 4px)` }}
              >
                <Still hue={stillHue[a.scheduleBlockId] ?? 200} className="absolute inset-0" />
                <div className="absolute inset-0 bg-linear-to-r from-black/80 via-black/40 to-transparent" />
                <div className="relative flex h-full flex-col justify-end px-2.5 pb-1.5">
                  <span className="text-static-0 truncate text-xs font-semibold [text-shadow:0_1px_6px_rgb(0_0_0/.9)]">
                    {programmeName(a)}
                  </span>
                  {!touch && (
                    <span className="text-static-300 truncate font-mono text-[10px]">{clock(a.startMs)}</span>
                  )}
                </div>
              </div>
            );
          })}
          {/* What's been watched: a warm wash up to the playhead, under a glowing playhead + knob. */}
          <div
            className="from-signal/0 to-signal/22 pointer-events-none absolute inset-y-0 left-0 rounded-l-md bg-linear-to-r"
            style={{ width: `${head}%` }}
          />
          <div
            className="bg-signal pointer-events-none absolute -top-2 bottom-0 z-30 w-0.5 -translate-x-1/2 shadow-[0_0_10px_rgb(255_176_32/.9)]"
            style={{ left: `${head}%` }}
          >
            <span className="bg-signal ring-signal/25 absolute -top-1.5 left-1/2 size-3.5 -translate-x-1/2 rounded-full shadow-[0_0_12px_rgb(255_176_32/.9)] ring-4" />
          </div>
          {lagMs > 0 && (
            <div
              className="bg-onair-300 pointer-events-none absolute inset-y-0 z-30 w-0.5 opacity-80"
              style={{ left: `${toPct(NOW)}%` }}
            />
          )}
        </div>
      </div>
      <div className="text-static-400 relative mt-1.5 h-3.5 px-1.5 font-mono text-[10px]">
        <span className="absolute left-0">−15m</span>
        <span
          className="text-onair-300 absolute -translate-x-1/2 font-semibold"
          style={{ left: `${toPct(NOW)}%` }}
        >
          LIVE
        </span>
        {[30, 60, 90].map((m) => (
          <span key={m} className="absolute -translate-x-1/2" style={{ left: `${toPct(NOW + m * MIN)}%` }}>
            {clock(NOW + m * MIN)}
          </span>
        ))}
      </div>
    </div>
  );
};

export { FilmstripBar, HoverBar };
