// PROTOTYPE — throwaway (#2088). Variant B — "Lower third": no top bar. Channel + programme live in one
// broadcast lower-third band bottom-left (also the station-ID card); a slim transport row sits under it;
// the past/future strip is revealed on demand; every sheet opens as ONE full-height right-hand panel.
import {
  Captions,
  Check,
  ChevronDown,
  ChevronRight,
  ChevronUp,
  Copy,
  Info,
  Keyboard,
  List,
  Maximize,
  Pause,
  Play,
  RotateCcw,
  RotateCw,
  Settings,
  Star,
  StepBack,
  Tv,
  Volume2,
  X,
} from "lucide-react";
import type { ReactNode } from "react";
import { TunerLoader } from "@/components/loomarr/shell/tuner-loader";
import { LiveIndicator } from "@/components/ui/video-player/live-indicator";
import { FilmstripBar, HoverBar } from "./preview-bar";
import {
  airings,
  audio,
  channel,
  channels,
  commercial,
  MIN,
  NOW,
  playbackInfo,
  quality,
  subtitles,
} from "./prototype-data";
import { PrototypePicture } from "./prototype-picture";
import type { VariantProps, WatchState } from "./prototype-state";

// Which playbar this B renders: the original revealed strip, or one of the two preview bars.
type Bar = "strip" | "filmstrip" | "hover";

const PANEL_STATES: readonly WatchState[] = ["info", "settings", "playback-info", "drawer", "help"];
const BEHIND_LAG_S = 134;

const current = airings[0];
const upNext = airings[2];
const programmeLine = `${current.series} · S${current.season} E${current.episode} "${current.title}"`;
const clock = (ms: number) => new Date(ms).toLocaleTimeString([], { hour: "numeric", minute: "2-digit" });
const elapsedMin = Math.round((NOW - current.startMs) / MIN);
const totalMin = Math.round((current.stopMs - current.startMs) / MIN);

const HELP_ROWS: [string, string][] = [
  ["Space / K", "Pause · play"],
  ["← / →", "Back / forward 10 s"],
  ["L", "Go live"],
  ["↑ / ↓  PgUp / PgDn", "Channel up / down"],
  ["0–9  Enter", "Tune a channel number"],
  ["G", "Channels"],
  ["M", "Mute"],
  ["− / =", "Volume down / up"],
  ["F", "Fullscreen"],
  ["P", "Picture-in-picture"],
  ["C", "Subtitles"],
  ["I", "Info"],
  ["?", "This sheet"],
  ["Esc", "Close / leave"],
];

const focusRing = "focus-visible:ring-signal outline-none focus-visible:ring-2";

const IconBtn = ({
  label,
  children,
  touch,
  active,
  disabled,
}: {
  label: string;
  children: ReactNode;
  touch: boolean;
  active?: boolean;
  disabled?: boolean;
}) => (
  <button
    type="button"
    aria-label={label}
    title={label}
    aria-disabled={disabled}
    className={`${focusRing} inline-flex shrink-0 cursor-pointer items-center justify-center rounded-md transition-colors ${
      touch ? "size-11" : "size-9"
    } ${active ? "bg-signal/20 text-signal" : "text-static-100 hover:bg-static-50/10"} ${disabled ? "opacity-35" : ""}`}
  >
    {children}
  </button>
);

const _Skip = ({ dir, touch, disabled }: { dir: "back" | "fwd"; touch: boolean; disabled?: boolean }) => (
  <IconBtn
    label={dir === "back" ? "Back 10 seconds" : "Forward 10 seconds"}
    touch={touch}
    disabled={disabled}
  >
    <span className="relative inline-flex items-center justify-center">
      {dir === "back" ? <RotateCcw className="size-5" /> : <RotateCw className="size-5" />}
      <span className="absolute font-mono text-[8px] font-bold">10</span>
    </span>
  </IconBtn>
);

const ghostBtn = `${focusRing} whitespace-nowrap border-static-500/60 text-static-50 hover:bg-static-50/10 inline-flex cursor-pointer items-center gap-1.5 rounded-md border px-3 text-xs font-semibold`;

// The lower-third band: number block + identity + programme line + thin progress. Doubles as the
// station-ID card (fuller: bigger block, era/place line).
const Band = ({ state, compact }: { state: WatchState; compact: boolean }) => {
  const stationId = state === "station-id";
  const isAds = state === "commercials";
  const behind = state === "behind";
  const pct = isAds ? 40 : Math.round((elapsedMin / totalMin) * 100);
  return (
    <div className="flex max-w-[60rem] items-stretch rounded-lg shadow-[0_22px_48px_-14px_rgb(0_0_0/.95)]">
      <div
        className={`from-signal to-signal-400 text-static-950 flex shrink-0 flex-col items-center justify-center rounded-l-lg bg-linear-to-b font-mono shadow-[inset_0_1px_0_rgb(255_255_255/.45),inset_0_-2px_0_rgb(0_0_0/.15)] ${
          stationId ? "w-24" : compact ? "w-12" : "w-16"
        }`}
      >
        <span className="text-[9px] leading-none font-bold tracking-[0.2em]">CH</span>
        <span
          className={`leading-none font-bold ${stationId ? "text-5xl" : compact ? "text-2xl" : "text-4xl"}`}
        >
          {channel.number}
        </span>
      </div>
      <div className="from-static-900/85 to-static-950/90 min-w-0 flex-1 rounded-r-lg bg-linear-to-b px-4 py-2.5 shadow-[inset_0_1px_0_rgb(255_255_255/.08)] ring-1 ring-white/10 backdrop-blur-xl">
        <div className="text-static-300 truncate font-mono text-[11px] tracking-[0.14em] uppercase">
          CH {channel.number} · {channel.name}
          {stationId && ` · ${channel.era} · ${channel.place}`}
        </div>
        <div
          className={`truncate leading-tight font-semibold ${stationId ? "mt-1 text-xl" : compact ? "text-sm" : "text-lg"}`}
        >
          {isAds ? `Commercials · ${commercial.clipTitle}` : programmeLine}
        </div>
        <div className="mt-2 flex items-center gap-3">
          <div className="bg-static-700 h-1 flex-1 overflow-hidden rounded-full">
            <div className={`h-full ${isAds ? "bg-tune-300" : "bg-signal"}`} style={{ width: `${pct}%` }} />
          </div>
          <span className="text-static-300 shrink-0 font-mono text-[11px] tabular-nums">
            {isAds
              ? "Break · 3 min"
              : behind
                ? `${elapsedMin - 2}m in · 2m 14s behind`
                : `${elapsedMin}m in · ${totalMin - elapsedMin}m left`}
          </span>
        </div>
      </div>
    </div>
  );
};

// ---- the revealed strip: 15 min of seekable past, then the (hover-only) future --------------------

interface Piece {
  label: string;
  from: number;
  to: number;
  ads: boolean;
}
const WINDOW_PAST = 15;
const WINDOW_FUTURE = 45;
const toPct = (m: number) => ((m + WINDOW_PAST) / (WINDOW_PAST + WINDOW_FUTURE)) * 100;

const pieces: Piece[] = (() => {
  const out: Piece[] = [{ label: "Earlier", from: -WINDOW_PAST, to: -9, ads: false }];
  for (const a of airings) {
    const from = Math.max(-WINDOW_PAST, (a.startMs - NOW) / MIN);
    const to = Math.min(WINDOW_FUTURE, (a.stopMs - NOW) / MIN);
    if (to <= from) continue;
    const label = a.kind === "filler" ? "Ads" : (a.series ?? a.title ?? "");
    const ads = a.kind === "filler";
    if (from < 0 && to > 0) {
      out.push({ label, from, to: 0, ads }, { label, from: 0, to, ads });
    } else {
      out.push({ label, from, to, ads });
    }
  }
  return out;
})();

const Strip = ({ lagMin, touch }: { lagMin: number; touch: boolean }) => {
  const head = toPct(-lagMin);
  return (
    <div className="bg-static-950/80 border-static-600/60 rounded-md border px-3 pt-2 pb-1.5 backdrop-blur-md">
      <div className="relative h-8">
        {pieces.map((p) => {
          const past = p.to <= 0;
          const cls = `absolute inset-y-0 overflow-hidden border-r border-black/60 px-1.5 text-left text-[10px] leading-8 whitespace-nowrap ${
            p.ads
              ? "bg-static-500/50"
              : past
                ? "bg-signal/35 text-static-50"
                : "bg-static-700/50 text-static-100"
          }`;
          const style = { left: `${toPct(p.from)}%`, width: `${toPct(p.to) - toPct(p.from)}%` };
          return past ? (
            <button
              key={`${p.label}-${p.from}`}
              type="button"
              aria-label={`Seek to ${p.label}`}
              className={`${cls} ${focusRing} hover:bg-signal/55 cursor-pointer`}
              style={style}
            >
              {p.label}
            </button>
          ) : (
            <div key={`${p.label}-${p.from}`} className={`${cls} group/f`} style={style} title={p.label}>
              <span className="opacity-0 transition-opacity group-hover/f:opacity-100 hover:opacity-100">
                {p.label}
              </span>
            </div>
          );
        })}
        <div className="bg-signal absolute -inset-y-1 w-0.5" style={{ left: `${head}%` }} />
        <div className="bg-onair-300 absolute -inset-y-1 w-0.5 opacity-60" style={{ left: `${toPct(0)}%` }} />
      </div>
      <div className="text-static-300 relative mt-1.5 h-4 font-mono text-[10px]">
        {[-10, -5, 15, 30].map((m) => (
          <span key={m} className="absolute -translate-x-1/2" style={{ left: `${toPct(m)}%` }}>
            {m < 0 ? `−${-m}m` : clock(NOW + m * MIN)}
          </span>
        ))}
        <span
          className="text-onair-300 absolute -translate-x-1/2 font-semibold"
          style={{ left: `${toPct(0)}%` }}
        >
          LIVE
        </span>
      </div>
      <span className="sr-only">
        Past 15 minutes can be sought; future is preview only{touch ? "" : " (hover)"}.
      </span>
    </div>
  );
};

// ---- the slim transport row ----------------------------------------------------------------------

const Transport = ({ state, touch }: { state: WatchState; touch: boolean }) => {
  const behind = state === "behind";
  // One vocabulary: every control lives in a lit glass capsule; capsules group by job (transport ·
  // live state · volume | track & view options · window). Play is the one primary, solid control.
  return (
    <div className="flex items-center gap-2.5">
      <div className={capsule}>
        <ToolBtn label="Back 10 seconds" keyHint="←" touch={touch}>
          <SkipGlyph dir="back" />
        </ToolBtn>
        <button
          type="button"
          aria-label={behind ? "Play" : "Pause"}
          title={behind ? "Play  (Space)" : "Pause  (Space)"}
          className={`${focusRing} bg-static-0 text-static-950 mx-0.5 inline-flex cursor-pointer items-center justify-center rounded-full shadow-[inset_0_-2px_0_rgb(0_0_0/.12),0_6px_18px_-6px_rgb(255_255_255/.35)] transition-transform active:scale-95 ${
            touch ? "size-12" : "size-11"
          }`}
        >
          {behind ? (
            <Play className="size-5 translate-x-px fill-current" strokeWidth={0} />
          ) : (
            <Pause className="size-5 fill-current" strokeWidth={0} />
          )}
        </button>
        <ToolBtn label="Forward 10 seconds" keyHint="→" touch={touch} disabled={!behind}>
          <SkipGlyph dir="fwd" />
        </ToolBtn>
      </div>
      <div className={`${capsule} pr-1`}>
        <LiveIndicator
          state={
            behind
              ? {
                  mode: "paused",
                  lagSeconds: BEHIND_LAG_S,
                  viewerTimeMs: NOW - BEHIND_LAG_S * 1000,
                  noticeRevision: 0,
                }
              : { mode: "live", lagSeconds: 0, viewerTimeMs: NOW, noticeRevision: 0 }
          }
          onGoLive={() => undefined}
        />
        <button
          type="button"
          className={`${focusRing} text-static-100 hover:text-static-0 inline-flex cursor-pointer items-center gap-1.5 rounded-full px-3 text-xs font-medium whitespace-nowrap transition-colors hover:bg-white/10 ${
            touch ? "h-10" : "h-8"
          }`}
        >
          <StepBack className="size-3.5" strokeWidth={2} />
          Start over
        </button>
      </div>
      {!touch && (
        <div className={`${capsule} pr-3`}>
          <ToolBtn label="Mute" keyHint="M" touch={touch}>
            <Volume2 className={glyph} strokeWidth={1.75} />
          </ToolBtn>
          <div className="relative h-1 w-20 rounded-full bg-white/15">
            <div className="from-static-200 to-static-0 h-full w-[70%] rounded-full bg-linear-to-r" />
            <span className="bg-static-0 absolute top-1/2 left-[70%] size-2.5 -translate-x-1/2 -translate-y-1/2 rounded-full shadow" />
          </div>
        </div>
      )}
      <div className="flex-1" />
      <div className={capsule}>
        <ToolBtn label="Subtitles" keyHint="C" touch={touch}>
          <Captions className={glyph} strokeWidth={1.75} />
        </ToolBtn>
        <ToolBtn label="Settings" touch={touch}>
          <Settings className={glyph} strokeWidth={1.75} />
        </ToolBtn>
        <ToolBtn label="Info" keyHint="I" touch={touch}>
          <Info className={glyph} strokeWidth={1.75} />
        </ToolBtn>
        <ToolBtn label="Channels" keyHint="G" touch={touch}>
          <List className={glyph} strokeWidth={1.75} />
        </ToolBtn>
      </div>
      <div className={capsule}>
        {!touch && (
          <ToolBtn label="Fullscreen" keyHint="F" touch={touch}>
            <Maximize className={glyph} strokeWidth={1.75} />
          </ToolBtn>
        )}
        <ToolBtn label="Leave" keyHint="Esc" touch={touch}>
          <X className={glyph} strokeWidth={1.75} />
        </ToolBtn>
      </div>
    </div>
  );
};

// The transport's shared pieces: a lit glass capsule, one icon size/weight, and a tool button with
// a soft hover well, an amber active dot (not a filled square), and a tooltip naming its key.
const capsule =
  "flex shrink-0 items-center gap-0.5 rounded-full bg-linear-to-b from-white/[.10] to-white/[.03] p-1 shadow-[inset_0_1px_0_rgb(255_255_255/.10),0_10px_26px_-12px_rgb(0_0_0/.95)] ring-1 ring-white/10 backdrop-blur-xl";
const glyph = "size-[18px]";

const ToolBtn = ({
  label,
  keyHint,
  children,
  touch,
  active,
  disabled,
}: {
  label: string;
  keyHint?: string;
  children: ReactNode;
  touch: boolean;
  active?: boolean;
  disabled?: boolean;
}) => (
  <button
    type="button"
    aria-label={label}
    aria-disabled={disabled}
    aria-pressed={active}
    className={`${focusRing} group/tool relative inline-flex shrink-0 cursor-pointer items-center justify-center rounded-full transition-colors ${
      touch ? "size-11" : "size-9"
    } ${active ? "text-signal bg-signal/10" : "text-static-200 hover:text-static-0 hover:bg-white/10"} ${
      disabled ? "pointer-events-none opacity-30" : ""
    }`}
  >
    {children}
    {active && <span className="bg-signal absolute bottom-1 size-1 rounded-full" />}
    {!touch && (
      <span className="bg-static-950/95 text-static-100 pointer-events-none absolute bottom-full mb-2.5 flex translate-y-1 items-center gap-1.5 rounded-md px-2 py-1 text-[11px] whitespace-nowrap opacity-0 shadow-lg ring-1 ring-white/10 transition-all group-hover/tool:translate-y-0 group-hover/tool:opacity-100">
        {label}
        {keyHint && (
          <kbd className="text-static-300 rounded bg-white/10 px-1 font-mono text-[10px]">{keyHint}</kbd>
        )}
      </span>
    )}
  </button>
);

// The 10-second skip glyph: the arrow with its number set INSIDE the circle, legibly.
const SkipGlyph = ({ dir }: { dir: "back" | "fwd" }) => (
  <span className="relative inline-flex size-[22px] items-center justify-center">
    {dir === "back" ? (
      <RotateCcw className="absolute inset-0 size-[22px]" strokeWidth={1.6} />
    ) : (
      <RotateCw className="absolute inset-0 size-[22px]" strokeWidth={1.6} />
    )}
    <span className="font-mono text-[8.5px] leading-none font-bold">10</span>
  </span>
);

// ---- panel contents ------------------------------------------------------------------------------

const SectionLabel = ({ children }: { children: ReactNode }) => (
  <h3 className="text-static-300 mt-5 mb-1.5 font-mono text-[11px] tracking-[0.16em] uppercase first:mt-0">
    {children}
  </h3>
);

const Option = ({ label, selected, hint }: { label: string; selected?: boolean; hint?: string }) => (
  <button
    type="button"
    className={`${focusRing} hover:bg-static-50/8 flex w-full cursor-pointer items-center gap-3 rounded-md px-3 py-2.5 text-left text-sm ${
      selected ? "text-signal" : "text-static-100"
    }`}
  >
    <span className="flex size-4 shrink-0 items-center justify-center">
      {selected && <Check className="size-4" />}
    </span>
    <span className="flex-1">{label}</span>
    {hint && <span className="text-static-400 font-mono text-xs">{hint}</span>}
  </button>
);

const InfoBody = () => (
  <div>
    <div className="relative aspect-video overflow-hidden rounded-md">
      <PrototypePicture />
    </div>
    <h3 className="mt-4 text-lg leading-tight font-semibold">{programmeLine}</h3>
    <p className="text-static-300 mt-1 text-sm">
      {current.year} · {current.rating} · {current.genres?.join(" · ")}
    </p>
    <p className="text-static-100 mt-3 text-sm leading-relaxed">{current.description}</p>
    <div className="mt-4">
      <div className="bg-static-700 h-1.5 overflow-hidden rounded-full">
        <div
          className="bg-signal h-full"
          style={{ width: `${Math.round((elapsedMin / totalMin) * 100)}%` }}
        />
      </div>
      <div className="text-static-300 mt-1.5 flex justify-between font-mono text-xs">
        <span>{clock(current.startMs)}</span>
        <span>
          {elapsedMin}m in · {totalMin - elapsedMin}m left
        </span>
        <span>{clock(current.stopMs)}</span>
      </div>
    </div>
    <div className="border-static-600 mt-5 border-t pt-4">
      <div className="text-tune-300 font-mono text-[11px] tracking-[0.16em] uppercase">Up next</div>
      <div className="mt-1 text-sm font-semibold">
        Up next · {clock(upNext.startMs)} · {upNext.series}
      </div>
      <div className="text-static-300 text-xs">
        S{upNext.season} E{upNext.episode} "{upNext.title}"
      </div>
    </div>
  </div>
);

const SettingsBody = () => (
  <div>
    <SectionLabel>Quality</SectionLabel>
    <Option label={`Auto · ${quality.current}`} selected={quality.auto} />
    {quality.rungs.map((r) => (
      <Option key={r} label={r} />
    ))}
    <SectionLabel>Audio</SectionLabel>
    {audio.options.map((o) => (
      <Option key={o} label={o} selected={o === audio.current} />
    ))}
    <SectionLabel>Subtitles</SectionLabel>
    {subtitles.options.map((o) => (
      <Option key={o} label={o} selected={o === subtitles.current} />
    ))}
    <div className="border-static-600 mt-4 border-t pt-2">
      <button
        type="button"
        className={`${focusRing} hover:bg-static-50/8 flex w-full cursor-pointer items-center gap-3 rounded-md px-3 py-2.5 text-left text-sm`}
      >
        <Info className="text-static-300 size-4" />
        <span className="flex-1">Playback info</span>
        <ChevronRight className="text-static-400 size-4" />
      </button>
    </div>
  </div>
);

const InfoRows = ({ rows }: { rows: readonly (readonly [string, string])[] }) => (
  <dl className="text-sm">
    {rows.map(([k, v]) => (
      <div key={k} className="border-static-700/70 flex justify-between gap-4 border-b py-2">
        <dt className="text-static-300 shrink-0">{k}</dt>
        <dd className="text-static-50 text-right font-mono text-xs leading-5">{v}</dd>
      </div>
    ))}
  </dl>
);

const PlaybackInfoBody = () => (
  <div>
    <SectionLabel>Stream</SectionLabel>
    <InfoRows rows={playbackInfo.viewer} />
    <div className="mt-5 mb-1.5 flex items-center gap-2">
      <h3 className="text-static-300 font-mono text-[11px] tracking-[0.16em] uppercase">Admin</h3>
      <span className="bg-signal/20 text-signal rounded px-1.5 py-0.5 font-mono text-[10px] font-bold">
        ADMIN ONLY
      </span>
    </div>
    <InfoRows rows={playbackInfo.admin} />
    <button type="button" className={`${ghostBtn} mt-4 h-10 w-full justify-center`}>
      <Copy className="size-4" />
      Copy stream URL
    </button>
  </div>
);

const DrawerBody = () => (
  <ul className="-mx-2">
    {channels.map((c) => {
      const here = c.number === channel.number;
      return (
        <li key={c.number}>
          <button
            type="button"
            aria-current={here ? "true" : undefined}
            className={`${focusRing} hover:bg-static-50/8 flex w-full cursor-pointer items-center gap-3 rounded-md px-2 py-2.5 text-left ${
              here ? "bg-signal/12" : ""
            }`}
          >
            <span
              className={`flex h-9 w-11 shrink-0 items-center justify-center rounded font-mono text-base font-bold ${
                here ? "bg-signal text-static-950" : "bg-static-800 text-static-100"
              }`}
            >
              {c.number}
            </span>
            <span className="min-w-0 flex-1">
              <span className="block truncate text-sm font-semibold">{c.name}</span>
              <span className="text-static-300 block truncate text-xs">
                {here ? "Watching now · " : "On now · "}
                {c.now}
              </span>
            </span>
            <Star
              className={`size-4 shrink-0 ${c.favourite ? "fill-signal text-signal" : "text-static-500"}`}
              aria-label={c.favourite ? "Favourite" : "Not a favourite"}
            />
          </button>
        </li>
      );
    })}
  </ul>
);

const HelpBody = () => (
  <dl className="text-sm">
    {HELP_ROWS.map(([k, v]) => (
      <div key={k} className="border-static-700/70 flex items-center justify-between gap-3 border-b py-2">
        <dt>
          <kbd className="bg-static-800 border-static-600 text-static-50 rounded border px-2 py-0.5 font-mono text-xs">
            {k}
          </kbd>
        </dt>
        <dd className="text-static-200 text-right">{v}</dd>
      </div>
    ))}
  </dl>
);

const PANELS: Partial<Record<WatchState, { title: string; icon: ReactNode; body: ReactNode }>> = {
  info: { title: "Info", icon: <Info className="size-4" />, body: <InfoBody /> },
  settings: { title: "Settings", icon: <Settings className="size-4" />, body: <SettingsBody /> },
  "playback-info": { title: "Playback info", icon: <Info className="size-4" />, body: <PlaybackInfoBody /> },
  drawer: { title: "Channels", icon: <List className="size-4" />, body: <DrawerBody /> },
  help: { title: "Keyboard shortcuts", icon: <Keyboard className="size-4" />, body: <HelpBody /> },
};

const PanelFrame = ({ state, touch, inline }: { state: WatchState; touch: boolean; inline?: boolean }) => {
  const p = PANELS[state];
  if (!p) return null;
  return (
    <aside
      aria-label={p.title}
      className={`bg-static-950/96 text-static-50 flex flex-col ${
        inline
          ? "min-h-0 flex-1"
          : `border-static-600 absolute inset-y-0 right-0 border-l shadow-2xl backdrop-blur-xl ${
              touch ? "w-[min(24rem,62%)]" : "w-[min(26rem,100%)]"
            }`
      }`}
    >
      <header className="border-static-700 flex shrink-0 items-center gap-2.5 border-b px-4 py-2.5">
        <span className="text-signal">{p.icon}</span>
        <h2 className="flex-1 text-sm font-semibold">{p.title}</h2>
        <span className="text-static-400 font-mono text-[10px]">
          CH {channel.number} · {channel.name}
        </span>
        <IconBtn label="Close" touch={touch}>
          <X className="size-5" />
        </IconBtn>
      </header>
      <div className="min-h-0 flex-1 overflow-y-auto px-4 py-4">{p.body}</div>
    </aside>
  );
};

// ---- full-picture states (tuning / error / off-air) ----------------------------------------------

const Cover = ({ state, touch }: { state: WatchState; touch: boolean }) => {
  if (state === "tuning") {
    return <TunerLoader channel={{ number: channel.number, name: channel.name }} heldFrame />;
  }
  if (state === "error") {
    return (
      <div className="bg-static-950/80 absolute inset-0 flex flex-col items-center justify-center gap-3 px-6 text-center backdrop-blur-sm">
        <p className="text-lg font-semibold">The stream stopped.</p>
        <p className="text-static-300 font-mono text-xs">
          CH {channel.number} · {channel.name}
        </p>
        <div className="mt-1 flex gap-2">
          <button
            type="button"
            className={`${focusRing} bg-signal text-static-950 inline-flex cursor-pointer items-center rounded-md px-4 text-sm font-bold ${touch ? "h-11" : "h-10"}`}
          >
            Retry channel
          </button>
          <button type="button" className={`${ghostBtn} ${touch ? "h-11" : "h-10"}`}>
            Leave
          </button>
        </div>
      </div>
    );
  }
  if (state === "off-air") {
    return (
      <div className="bg-static-950 absolute inset-0 flex flex-col items-center justify-center gap-2 px-6 text-center">
        <Tv className="text-static-500 size-10" />
        <p className="text-lg font-semibold">{channel.name} is off air</p>
        <p className="text-static-300 max-w-xs text-sm">
          This channel is paused, so there's nothing to watch.
        </p>
        <button type="button" className={`${ghostBtn} mt-2 ${touch ? "h-11" : "h-10"}`}>
          Leave
        </button>
      </div>
    );
  }
  return null;
};

// ---- desktop + landscape: the full-bleed overlay ------------------------------------------------

const Overlay = ({ state, touch, bar }: { state: WatchState; touch: boolean; bar: Bar }) => {
  const panel = PANEL_STATES.includes(state);
  const cover = state === "tuning" || state === "error" || state === "off-air";
  const showBand = !panel && !cover && state !== "idle";
  const showTransport = showBand && state !== "station-id";
  const stripOpen = bar === "strip" ? state === "behind" : true;
  return (
    <div className="text-static-50 relative size-full overflow-hidden bg-black">
      <PrototypePicture />
      <Cover state={state} touch={touch} />
      {panel && <div className="absolute inset-0 bg-black/60" />}
      {panel && <PanelFrame state={state} touch={touch} />}
      {showBand && (
        <div
          className={`absolute inset-x-0 bottom-0 bg-gradient-to-t from-black/85 via-black/45 to-transparent ${
            touch ? "space-y-1.5 px-4 pt-10 pb-2.5" : "space-y-3 px-8 pt-24 pb-5"
          }`}
        >
          {/* While a hover preview is open (every state that shows the bar, in this prototype) the band
              steps back a plane — dimmer, softer — so the card is unambiguously in front of it. */}
          <div
            className={
              bar === "hover" && showTransport && !touch
                ? "origin-bottom-left scale-[.985] opacity-40 blur-[1px] transition-all duration-300"
                : "transition-all duration-300"
            }
          >
            <Band state={state} compact={touch} />
          </div>
          {showTransport && (
            <>
              {bar === "strip" && stripOpen && <Strip lagMin={BEHIND_LAG_S / 60} touch={touch} />}
              {bar === "filmstrip" && <FilmstripBar state={state} touch={touch} />}
              {bar === "hover" && <HoverBar state={state} touch={touch} />}
              <Transport state={state} touch={touch} />
            </>
          )}
        </div>
      )}
    </div>
  );
};

// ---- portrait: picture on top, panel below ------------------------------------------------------

const Portrait = ({ state, bar }: { state: WatchState; bar: Bar }) => {
  const panel = PANEL_STATES.includes(state);
  const behind = state === "behind";
  const ads = state === "commercials";
  return (
    <div className="bg-static-950 text-static-50 flex size-full flex-col">
      <div className="relative aspect-video shrink-0 overflow-hidden bg-black">
        <PrototypePicture />
        <Cover state={state} touch />
        {state === "station-id" && (
          <div className="absolute inset-x-0 bottom-0 bg-gradient-to-t from-black/85 to-transparent px-3 pt-8 pb-2.5">
            <Band state={state} compact />
          </div>
        )}
      </div>
      {panel ? (
        <PanelFrame state={state} touch inline />
      ) : (
        <div className="flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto px-4 pt-3 pb-5">
          <div className="flex items-center justify-between gap-2">
            <div className={capsule}>
              <ToolBtn label="Leave" touch>
                <X className={glyph} strokeWidth={1.75} />
              </ToolBtn>
            </div>
            <LiveIndicator
              state={
                behind
                  ? {
                      mode: "paused",
                      lagSeconds: BEHIND_LAG_S,
                      viewerTimeMs: NOW - BEHIND_LAG_S * 1000,
                      noticeRevision: 0,
                    }
                  : { mode: "live", lagSeconds: 0, viewerTimeMs: NOW, noticeRevision: 0 }
              }
              onGoLive={() => undefined}
            />
          </div>
          <section>
            <div className="text-static-300 font-mono text-[11px] tracking-[0.14em] uppercase">
              CH {channel.number} · {channel.name}
            </div>
            <div className="mt-1 text-base leading-snug font-semibold">
              {ads ? `Commercials · ${commercial.clipTitle}` : programmeLine}
            </div>
            <div className="bg-static-700 mt-2.5 h-1 overflow-hidden rounded-full">
              <div
                className={`h-full ${ads ? "bg-tune-300" : "bg-signal"}`}
                style={{ width: `${ads ? 40 : Math.round((elapsedMin / totalMin) * 100)}%` }}
              />
            </div>
            <div className="text-static-300 mt-1.5 font-mono text-[11px]">
              {behind ? "2m 14s behind live · paused" : `${elapsedMin}m in · ${totalMin - elapsedMin}m left`}
            </div>
            {/* The same capsules as the overlay transport: one primary, grouped by job. */}
            <div className="mt-3 flex items-center gap-2.5">
              <div className={capsule}>
                <ToolBtn label="Back 10 seconds" touch>
                  <SkipGlyph dir="back" />
                </ToolBtn>
                <button
                  type="button"
                  aria-label={behind ? "Play" : "Pause"}
                  className={`${focusRing} bg-static-0 text-static-950 mx-0.5 inline-flex size-12 cursor-pointer items-center justify-center rounded-full shadow-[inset_0_-2px_0_rgb(0_0_0/.12),0_6px_18px_-6px_rgb(255_255_255/.35)]`}
                >
                  {behind ? (
                    <Play className="size-5 translate-x-px fill-current" strokeWidth={0} />
                  ) : (
                    <Pause className="size-5 fill-current" strokeWidth={0} />
                  )}
                </button>
                <ToolBtn label="Forward 10 seconds" touch disabled={!behind}>
                  <SkipGlyph dir="fwd" />
                </ToolBtn>
              </div>
              <div className="flex-1" />
              <div className={capsule}>
                <button
                  type="button"
                  className={`${focusRing} text-static-100 inline-flex h-11 cursor-pointer items-center gap-1.5 rounded-full px-3.5 text-xs font-medium whitespace-nowrap hover:bg-white/10`}
                >
                  <StepBack className="size-3.5" strokeWidth={2} />
                  Start over
                </button>
              </div>
            </div>
          </section>
          {bar === "strip" ? (
            <section className="border-static-700 border-t pt-3">
              <div className="text-tune-300 font-mono text-[11px] tracking-[0.16em] uppercase">Up next</div>
              <div className="mt-1 text-sm font-semibold">
                {clock(upNext.startMs)} · {upNext.series}
              </div>
            </section>
          ) : (
            // Touch has no hover, so both preview bars fall back to the scrollable filmstrip here.
            <section className="border-static-700 border-t pt-3">
              <div className="text-tune-300 mb-2 font-mono text-[11px] tracking-[0.16em] uppercase">
                Coming up
              </div>
              <FilmstripBar state={state} touch />
            </section>
          )}
          <section className="mt-auto flex items-center gap-2">
            <div className={capsule}>
              <ToolBtn label="Channel up" touch>
                <ChevronUp className={glyph} strokeWidth={1.75} />
              </ToolBtn>
              <ToolBtn label="Channel down" touch>
                <ChevronDown className={glyph} strokeWidth={1.75} />
              </ToolBtn>
              <button
                type="button"
                className={`${focusRing} text-static-100 inline-flex h-11 cursor-pointer items-center rounded-full px-3 text-xs font-medium hover:bg-white/10`}
              >
                Previous
              </button>
            </div>
            <div className={capsule}>
              <ToolBtn label="Favourite" touch active>
                <Star className={`${glyph} fill-signal`} strokeWidth={1.75} />
              </ToolBtn>
            </div>
            <div className="flex-1" />
            <div className={capsule}>
              <ToolBtn label="Settings" touch>
                <Settings className={glyph} strokeWidth={1.75} />
              </ToolBtn>
              <ToolBtn label="Info" touch>
                <Info className={glyph} strokeWidth={1.75} />
              </ToolBtn>
            </div>
          </section>
        </div>
      )}
    </div>
  );
};

const WithBar = ({ state, device, bar }: VariantProps & { bar: Bar }) =>
  device === "portrait" ? (
    <Portrait state={state} bar={bar} />
  ) : (
    <Overlay state={state} touch={device === "landscape"} bar={bar} />
  );

const VariantB = (props: VariantProps) => <WithBar {...props} bar="strip" />;
const VariantB1 = (props: VariantProps) => <WithBar {...props} bar="filmstrip" />;
const VariantB2 = (props: VariantProps) => <WithBar {...props} bar="hover" />;

export { VariantB, VariantB1, VariantB2 };
