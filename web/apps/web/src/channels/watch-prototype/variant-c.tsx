// PROTOTYPE — throwaway (#2088). Variant C — "Side rail". While nothing is open the picture is a
// full-bleed overlay player. Info / settings / playback-info / drawer / help open a persistent right
// RAIL (tabs: Info · Up next · Channels · Settings, help as "?") and the picture shrinks left. On a
// phone portrait the rail IS the panel under the picture.
import {
  ChevronDown,
  ChevronLeft,
  ChevronUp,
  Copy,
  Info,
  Keyboard,
  Maximize,
  Pause,
  Play,
  RotateCcw,
  RotateCw,
  Settings,
  Star,
  Volume2,
  X,
} from "lucide-react";
import type { ReactNode } from "react";
import { TunerLoader } from "@/components/loomarr/shell/tuner-loader";
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
import type { Device, VariantProps, WatchState } from "./prototype-state";

type Tab = "info" | "upnext" | "channels" | "settings" | "help";

const BEHIND_MS = (2 * 60 + 14) * 1000;
const WINDOW_START = NOW - 15 * MIN;
const WINDOW_END = NOW + 75 * MIN;

const RAIL_TAB: Partial<Record<WatchState, Tab>> = {
  info: "info",
  settings: "settings",
  "playback-info": "settings",
  drawer: "channels",
  help: "help",
};

const SHORTCUTS: [string, string][] = [
  ["Space / K", "Pause · play"],
  ["← / →", "10 s back · forward"],
  ["L", "Go live"],
  ["↑ ↓ / PgUp PgDn", "Channel up · down"],
  ["0–9 Enter", "Tune a number"],
  ["G", "Channels"],
  ["M", "Mute"],
  ["− / =", "Volume"],
  ["F", "Fullscreen"],
  ["P", "Picture-in-picture"],
  ["C", "Subtitles"],
  ["I", "Info"],
  ["?", "This sheet"],
  ["Esc", "Close / leave"],
];

const clock = (ms: number) => new Date(ms).toLocaleTimeString([], { hour: "numeric", minute: "2-digit" });
const mins = (a: number, b: number) => Math.round((b - a) / MIN);
const pct = (ms: number) =>
  `${Math.min(100, Math.max(0, ((ms - WINDOW_START) / (WINDOW_END - WINDOW_START)) * 100))}%`;
const programmeLine = "Coastline Mysteries · S2 E4 “The Last Lighthouse”";

const current = airings[0];
const upNext = airings.slice(1);

const Identity = ({ state, large }: { state: WatchState; large?: boolean }) => (
  <div className="min-w-0">
    <div className={`text-static-100 truncate font-semibold ${large ? "text-lg" : "text-sm"}`}>
      {state === "commercials" ? (
        <>
          <span className="text-signal">Commercials</span> · {commercial.clipTitle}
        </>
      ) : (
        <>
          CH {channel.number} · {channel.name}
        </>
      )}
    </div>
    <div className="text-static-300 truncate text-xs">
      {state === "commercials" ? `CH ${channel.number} · ${channel.name}` : programmeLine}
    </div>
  </div>
);

const IconBtn = ({
  label,
  children,
  big,
  active,
  onClick,
}: {
  label: string;
  children: ReactNode;
  big?: boolean;
  active?: boolean;
  onClick?: () => void;
}) => (
  <button
    type="button"
    aria-label={label}
    title={label}
    onClick={onClick}
    className={`focus-visible:ring-signal text-static-100 flex shrink-0 items-center justify-center rounded-full outline-none hover:bg-white/15 focus-visible:ring-2 ${big ? "size-11" : "size-9"} ${active ? "bg-white/20" : ""}`}
  >
    {children}
  </button>
);

const Chip = ({ children, tone }: { children: ReactNode; tone?: "live" | "signal" }) => (
  <button
    type="button"
    className={`focus-visible:ring-signal inline-flex h-8 shrink-0 items-center gap-1.5 rounded-full border px-3 text-xs font-medium whitespace-nowrap outline-none focus-visible:ring-2 ${
      tone === "signal"
        ? "bg-signal border-signal text-static-950"
        : "text-static-100 border-white/25 bg-black/40 hover:bg-white/15"
    }`}
  >
    {children}
  </button>
);

// The mini-guide strip: 15 min of seekable past, the schedule ahead (hover only), the playhead.
const Strip = ({ behind }: { behind: boolean }) => {
  const head = behind ? NOW - BEHIND_MS : NOW;
  return (
    <div
      className="relative h-6 w-full"
      role="slider"
      tabIndex={0}
      aria-label="Mini guide"
      aria-valuenow={behind ? 134 : 0}
    >
      <div className="absolute inset-x-0 top-2.5 h-1.5 overflow-hidden rounded-full bg-white/10">
        {airings.map((a) => (
          <div
            key={a.scheduleBlockId}
            className={`absolute top-0 h-full border-r border-black/70 ${a.kind === "filler" ? "bg-white/30" : "bg-white/20"}`}
            style={{ left: pct(a.startMs), width: `calc(${pct(a.stopMs)} - ${pct(a.startMs)})` }}
          />
        ))}
        <div className="bg-signal/40 absolute top-0 h-full" style={{ left: 0, width: pct(NOW) }} />
        <div className="bg-signal absolute top-0 h-full" style={{ left: 0, width: pct(head) }} />
      </div>
      <div
        className="bg-signal absolute top-1 size-4 -translate-x-1/2 rounded-full shadow ring-2 ring-black/60"
        style={{ left: pct(head) }}
      />
      <span className="text-static-300 absolute -bottom-0.5 left-0 text-[10px]">−15 min</span>
      <span className="text-static-300 absolute -bottom-0.5 text-[10px]" style={{ left: pct(NOW) }}>
        <span className="relative -translate-x-1/2">now</span>
      </span>
    </div>
  );
};

const LivePill = ({ behind }: { behind: boolean }) =>
  behind ? (
    <span className="bg-signal/15 text-signal inline-flex h-8 items-center rounded-full px-3 font-mono text-xs">
      2m 14s behind live
    </span>
  ) : (
    <span className="border-onair-300/25 bg-onair-tint-15 text-onair-300 inline-flex h-8 items-center gap-1.5 rounded-full border px-3 font-mono text-xs tracking-widest uppercase">
      <span className="bg-onair-300 size-1.5 rounded-full" aria-hidden />
      Live
    </span>
  );

// Compact on-picture transport — stays usable while the rail is open.
const Transport = ({ behind, compact, touch }: { behind: boolean; compact: boolean; touch: boolean }) => (
  <div className="flex flex-col gap-1">
    <Strip behind={behind} />
    <div className="flex items-center gap-1.5">
      <IconBtn label="Back 10 seconds" big={touch}>
        <RotateCcw className="size-5" />
      </IconBtn>
      <IconBtn label={behind ? "Play" : "Pause"} big>
        {behind ? (
          <Play className="size-6" fill="currentColor" />
        ) : (
          <Pause className="size-6" fill="currentColor" />
        )}
      </IconBtn>
      <IconBtn label="Forward 10 seconds" big={touch}>
        <RotateCw className="size-5" />
      </IconBtn>
      <LivePill behind={behind} />
      <div className="flex-1" />
      {behind && <Chip tone="signal">Go live</Chip>}
      {(!behind || !compact) && <Chip>Start over</Chip>}
      {!compact && (
        <IconBtn label="Mute" big={touch}>
          <Volume2 className="size-5" />
        </IconBtn>
      )}
      <IconBtn label="Fullscreen" big={touch}>
        <Maximize className="size-5" />
      </IconBtn>
    </div>
  </div>
);

const UpNextList = () => (
  <ol className="flex flex-col gap-2">
    {upNext.map((a, i) => (
      <li
        key={a.scheduleBlockId}
        className={`rounded-lg border px-3 py-2 ${a.kind === "filler" ? "border-white/5 bg-white/[0.03]" : "border-white/10 bg-white/[0.07]"}`}
      >
        <div className="text-static-300 flex items-center justify-between text-xs">
          <span>{i === 0 ? `Up next · ${clock(a.startMs)}` : clock(a.startMs)}</span>
          <span>{mins(a.startMs, a.stopMs)} min</span>
        </div>
        <div
          className={`truncate text-sm ${a.kind === "filler" ? "text-static-300 italic" : "text-static-100 font-medium"}`}
        >
          {a.title}
        </div>
        {a.series && (
          <div className="text-static-300 truncate text-xs">
            {a.series} · S{a.season} E{a.episode}
          </div>
        )}
      </li>
    ))}
  </ol>
);

const InfoTab = () => (
  <div className="flex flex-col gap-3">
    <div className="relative aspect-video overflow-hidden rounded-lg">
      <PrototypePicture />
      <span className="text-static-100 absolute right-2 bottom-2 rounded bg-black/60 px-1.5 py-0.5 font-mono text-[10px]">
        {clock(current.startMs)} – {clock(current.stopMs)}
      </span>
    </div>
    <div>
      <div className="text-static-100 text-base font-semibold">{current.title}</div>
      <div className="text-static-300 text-xs">
        {current.series} · S{current.season} E{current.episode}
      </div>
    </div>
    <div className="text-static-200 flex flex-wrap items-center gap-2 text-xs">
      <span>{current.year}</span>
      <span className="rounded border border-white/30 px-1.5 py-px">{current.rating}</span>
      {current.genres?.map((g) => (
        <span key={g} className="rounded-full bg-white/10 px-2 py-0.5">
          {g}
        </span>
      ))}
    </div>
    <p className="text-static-200 text-sm leading-relaxed">{current.description}</p>
    <div>
      <div className="h-1 overflow-hidden rounded-full bg-white/15">
        <div className="bg-signal h-full" style={{ width: `${(9 / 22) * 100}%` }} />
      </div>
      <div className="text-static-300 mt-1 flex justify-between text-xs">
        <span>Started 9 min ago</span>
        <span>13 min left</span>
      </div>
    </div>
    <div className="border-t border-white/10 pt-3">
      <div className="text-static-300 text-xs">
        Up next · {clock(upNext[1].startMs)} · {upNext[1].title}
      </div>
    </div>
  </div>
);

const ChannelsTab = () => (
  <ul className="flex flex-col gap-1">
    {channels.map((c) => {
      const on = c.number === channel.number;
      return (
        <li
          key={c.number}
          className={`flex items-center gap-3 rounded-lg px-3 py-2 ${on ? "bg-signal/15 ring-signal ring-1" : "hover:bg-white/10"}`}
        >
          <span className="text-static-100 w-8 font-mono text-sm">{c.number}</span>
          <div className="min-w-0 flex-1">
            <div className="text-static-100 truncate text-sm font-medium">{c.name}</div>
            <div className="text-static-300 truncate text-xs">Now · {c.now}</div>
          </div>
          {c.favourite && <Star className="text-signal size-4" fill="currentColor" aria-label="Favourite" />}
        </li>
      );
    })}
  </ul>
);

const Group = ({ title, children }: { title: string; children: ReactNode }) => (
  <section className="flex flex-col gap-1">
    <h3 className="text-static-300 px-1 text-[11px] font-semibold tracking-widest uppercase">{title}</h3>
    {children}
  </section>
);

const Option = ({ label, selected, hint }: { label: string; selected: boolean; hint?: string }) => (
  <button
    type="button"
    className={`flex min-h-9 items-center justify-between rounded-md px-3 py-1.5 text-left text-sm ${selected ? "bg-signal/15 text-signal" : "text-static-100 hover:bg-white/10"}`}
  >
    <span>{label}</span>
    <span className="text-static-300 text-xs">{hint ?? (selected ? "✓" : "")}</span>
  </button>
);

const SettingsTab = () => (
  <div className="flex flex-col gap-4">
    <Group title="Quality">
      <Option label={`Auto · ${quality.current}`} selected={quality.auto} />
      {quality.rungs.map((r) => (
        <Option key={r} label={r} selected={false} />
      ))}
    </Group>
    <Group title="Audio">
      {audio.options.map((o) => (
        <Option key={o} label={o} selected={o === audio.current} />
      ))}
    </Group>
    <Group title="Subtitles">
      {subtitles.options.map((o) => (
        <Option key={o} label={o} selected={o === subtitles.current} />
      ))}
    </Group>
    <Group title="Diagnostics">
      <Option label="Playback info" selected={false} hint="›" />
    </Group>
  </div>
);

const InfoRows = ({ rows }: { rows: readonly (readonly [string, string])[] }) => (
  <dl className="flex flex-col">
    {rows.map(([k, v]) => (
      <div key={k} className="flex justify-between gap-3 border-b border-white/5 py-1.5 text-xs">
        <dt className="text-static-300 shrink-0">{k}</dt>
        <dd className="text-static-100 text-right font-mono">{v}</dd>
      </div>
    ))}
  </dl>
);

const PlaybackInfoView = () => (
  <div className="flex flex-col gap-4">
    <button type="button" className="text-static-200 flex items-center gap-1 text-xs">
      <ChevronLeft className="size-4" /> Settings
    </button>
    <Group title="Playback info">
      <InfoRows rows={playbackInfo.viewer} />
    </Group>
    <Group title="Admin">
      <InfoRows rows={playbackInfo.admin} />
      <button
        type="button"
        className="text-static-100 mt-2 inline-flex h-9 items-center justify-center gap-2 rounded-md border border-white/25 text-sm hover:bg-white/10"
      >
        <Copy className="size-4" /> Copy stream URL
      </button>
    </Group>
  </div>
);

const HelpTab = () => (
  <dl className="flex flex-col">
    {SHORTCUTS.map(([k, v]) => (
      <div key={k} className="flex items-center justify-between gap-3 border-b border-white/5 py-1.5 text-sm">
        <dt>
          <kbd className="text-static-100 rounded border border-white/25 bg-white/10 px-1.5 py-0.5 font-mono text-xs">
            {k}
          </kbd>
        </dt>
        <dd className="text-static-200 text-right text-xs">{v}</dd>
      </div>
    ))}
  </dl>
);

const TABS: { id: Tab; label: string }[] = [
  { id: "info", label: "Info" },
  { id: "upnext", label: "Up next" },
  { id: "channels", label: "Channels" },
  { id: "settings", label: "Settings" },
];

const Rail = ({
  tab,
  state,
  onClose,
  className = "",
  children,
}: {
  tab: Tab;
  state: WatchState;
  onClose?: () => void;
  className?: string;
  children?: ReactNode;
}) => (
  <aside
    aria-label="Channel rail"
    className={`bg-static-900 flex min-h-0 flex-col border-l border-white/10 ${className}`}
  >
    <div role="tablist" className="flex shrink-0 items-center gap-0.5 border-b border-white/10 px-2">
      {TABS.map((t) => (
        <button
          key={t.id}
          type="button"
          role="tab"
          aria-selected={tab === t.id}
          className={`h-10 border-b-2 px-2.5 text-xs font-medium whitespace-nowrap ${tab === t.id ? "border-signal text-static-100" : "text-static-300 border-transparent"}`}
        >
          {t.label}
        </button>
      ))}
      <div className="flex-1" />
      <button
        type="button"
        role="tab"
        aria-selected={tab === "help"}
        aria-label="Keyboard shortcuts"
        className={`flex size-8 items-center justify-center rounded-full ${tab === "help" ? "text-static-100 bg-white/20" : "text-static-300"}`}
      >
        <Keyboard className="size-4" />
      </button>
      {onClose && (
        <button
          type="button"
          aria-label="Close rail"
          onClick={onClose}
          className="text-static-300 flex size-8 items-center justify-center rounded-full"
        >
          <X className="size-4" />
        </button>
      )}
    </div>
    <div className="min-h-0 flex-1 overflow-y-auto p-3">
      {children ??
        (tab === "info" ? (
          <InfoTab />
        ) : tab === "upnext" ? (
          <UpNextList />
        ) : tab === "channels" ? (
          <ChannelsTab />
        ) : tab === "help" ? (
          <HelpTab />
        ) : state === "playback-info" ? (
          <PlaybackInfoView />
        ) : (
          <SettingsTab />
        ))}
    </div>
  </aside>
);

// Centered status cards drawn over the picture.
const Overlay = ({ state, small }: { state: WatchState; small: boolean }) => {
  if (state === "tuning") return <TunerLoader channel={channel} heldFrame />;
  if (state === "error")
    return (
      <div className="absolute inset-0 flex flex-col items-center justify-center gap-3 bg-black/70 p-4 text-center">
        <div className={`text-static-100 font-semibold ${small ? "text-base" : "text-xl"}`}>
          The stream stopped.
        </div>
        <button
          type="button"
          className="bg-signal text-static-950 h-10 rounded-full px-5 text-sm font-semibold"
        >
          Retry channel
        </button>
      </div>
    );
  if (state === "off-air")
    return (
      <div className="bg-static-950/90 absolute inset-0 flex flex-col items-center justify-center gap-1 p-4 text-center">
        <div className={`text-static-100 font-semibold ${small ? "text-base" : "text-xl"}`}>
          Coast 90s is off air
        </div>
        <div className="text-static-300 text-sm">
          This channel is paused, so there&apos;s nothing to watch.
        </div>
      </div>
    );
  if (state === "station-id")
    return (
      <div className="absolute bottom-[12%] left-[5%] flex max-w-[80%] items-center gap-4 rounded-xl border border-white/15 bg-black/70 p-4 backdrop-blur">
        <span className="text-signal font-mono text-4xl leading-none font-bold">{channel.number}</span>
        <div className="min-w-0">
          <div className="text-static-100 text-lg font-semibold">{channel.name}</div>
          <div className="text-static-300 text-xs">
            {channel.era} · {channel.place}
          </div>
          <div className="text-static-200 mt-1 truncate text-xs">{programmeLine}</div>
        </div>
      </div>
    );
  return null;
};

const CtlButtons = ({ stack }: { stack?: boolean }) => (
  <div className={`flex items-center gap-1 ${stack ? "" : ""}`}>
    <IconBtn label="Favourite" big>
      <Star className="text-signal size-5" fill="currentColor" />
    </IconBtn>
    <IconBtn label="Settings" big>
      <Settings className="size-5" />
    </IconBtn>
    <IconBtn label="Info" big>
      <Info className="size-5" />
    </IconBtn>
  </div>
);

const Phone = ({ state }: { state: WatchState }) => {
  const behind = state === "behind";
  const tab = RAIL_TAB[state] ?? "upnext";
  return (
    <div className="bg-static-950 flex h-full w-full flex-col">
      <div className="relative aspect-video w-full shrink-0 overflow-hidden bg-black">
        <PrototypePicture />
        <Overlay state={state} small />
      </div>
      <div className="flex shrink-0 items-center gap-2 px-3 py-2">
        <IconBtn label="Leave">
          <X className="size-5" />
        </IconBtn>
        <Identity state={state} />
      </div>
      <div className="shrink-0 px-3">
        <Strip behind={behind} />
        <div className="flex items-center gap-1">
          <IconBtn label="Back 10 seconds" big>
            <RotateCcw className="size-5" />
          </IconBtn>
          <IconBtn label="Play" big>
            {behind ? (
              <Play className="size-6" fill="currentColor" />
            ) : (
              <Pause className="size-6" fill="currentColor" />
            )}
          </IconBtn>
          <IconBtn label="Forward 10 seconds" big>
            <RotateCw className="size-5" />
          </IconBtn>
          <LivePill behind={behind} />
          <div className="flex-1" />
          {behind ? <Chip tone="signal">Go live</Chip> : <Chip>Start over</Chip>}
        </div>
      </div>
      <div className="flex shrink-0 items-center gap-1 px-3 py-1">
        <Chip>
          <ChevronUp className="size-4" /> Next
        </Chip>
        <Chip>
          <ChevronDown className="size-4" /> Previous
        </Chip>
        <div className="flex-1" />
        <CtlButtons />
      </div>
      <Rail tab={tab} state={state} className="flex-1 border-t border-l-0" />
    </div>
  );
};

const Wide = ({ state, device }: { state: WatchState; device: Device }) => {
  const railTab = RAIL_TAB[state];
  const open = railTab !== undefined;
  const hidden =
    state === "idle" ||
    state === "tuning" ||
    state === "station-id" ||
    state === "error" ||
    state === "off-air";
  const touch = device === "landscape";
  const behind = state === "behind";
  return (
    <div className="bg-static-950 flex h-full w-full">
      <div className="relative min-w-0 flex-1 overflow-hidden bg-black">
        <PrototypePicture />
        {!hidden && (
          <>
            <div className="absolute inset-x-0 top-0 flex items-center gap-2 bg-gradient-to-b from-black/80 to-transparent p-3 pb-8">
              <IconBtn label="Leave" big={touch}>
                <X className="size-5" />
              </IconBtn>
              <Identity state={state} large={!touch && !open} />
              <div className="flex-1" />
              {!open && (
                <>
                  <IconBtn label="Favourite" big={touch}>
                    <Star className="text-signal size-5" fill="currentColor" />
                  </IconBtn>
                  <IconBtn label="Channels" big={touch}>
                    <ChevronUp className="size-5" />
                  </IconBtn>
                  <IconBtn label="Info" big={touch}>
                    <Info className="size-5" />
                  </IconBtn>
                  <IconBtn label="Settings" big={touch}>
                    <Settings className="size-5" />
                  </IconBtn>
                </>
              )}
            </div>
            <div className="absolute inset-x-0 bottom-0 bg-gradient-to-t from-black/85 to-transparent p-3 pt-10">
              <Transport behind={behind} compact={open || touch} touch={touch} />
            </div>
          </>
        )}
        <Overlay state={state} small={touch} />
      </div>
      {open && (
        <Rail
          tab={railTab}
          state={state}
          onClose={() => undefined}
          className={`shrink-0 ${touch ? "w-[300px]" : "w-[360px]"}`}
        />
      )}
    </div>
  );
};

const VariantC = ({ state, device }: VariantProps) =>
  device === "portrait" ? <Phone state={state} /> : <Wide state={state} device={device} />;

export { VariantC };
