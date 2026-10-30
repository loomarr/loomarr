// PROTOTYPE — throwaway (#2088 step 0). Variant A — "Overlay classic": the Jellyfin/Emby shape.
// Everything overlays the picture: a top bar (back, identity, programme line), a bottom stack (the
// seekable mini-guide strip, then one transport row), and transient layers as popovers anchored to
// the control that opened them (⚙ bottom-right, info bottom-left, stats top-left, drawer right edge).
import {
  ArrowLeft,
  Captions,
  ChevronDown,
  ChevronUp,
  Info,
  Maximize,
  PictureInPicture2,
  Play,
  RotateCcw,
  RotateCw,
  Settings,
  Star,
  Volume2,
} from "lucide-react";
import type { ReactNode } from "react";
import { TunerLoader } from "@/components/loomarr/shell/tuner-loader";
import { LiveIndicator } from "@/components/ui/video-player/live-indicator";
import { cn } from "@/lib/utils";
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

const current = airings[0];
const next = airings[2];
const clock = (ms: number) => new Date(ms).toLocaleTimeString([], { hour: "numeric", minute: "2-digit" });
const programmeLine = `${current?.series} · S${current?.season} E${current?.episode} "${current?.title}"`;

// The strip spans 15 min behind live (seekable, the DVR window) and an hour ahead (hover-only).
const WINDOW_BACK = 15 * MIN;
const WINDOW_AHEAD = 60 * MIN;
const span = WINDOW_BACK + WINDOW_AHEAD;
const pct = (ms: number) => ((ms - (NOW - WINDOW_BACK)) / span) * 100;

const Strip = ({ behind }: { behind: boolean }) => {
  const playhead = behind ? NOW - 2.25 * MIN : NOW;
  return (
    <div className="relative">
      <div className="relative h-7">
        {/* The seekable past: the 15 min the stream still holds. */}
        <div
          className="bg-static-0/10 absolute inset-y-0 left-0 rounded-l-md"
          style={{ width: `${pct(NOW)}%` }}
        />
        {airings.map((a) => {
          const left = Math.max(0, pct(a.startMs));
          const right = Math.min(100, pct(a.stopMs));
          if (right <= 0 || left >= 100) return null;
          const brk = a.kind === "filler";
          return (
            <div
              key={a.scheduleBlockId}
              className={cn(
                "absolute inset-y-1 flex items-center overflow-hidden rounded px-2 text-[11px]",
                brk ? "bg-signal/25 text-signal" : "bg-static-0/15 text-static-100",
              )}
              style={{ left: `calc(${left}% + 1px)`, width: `calc(${right - left}% - 2px)` }}
            >
              <span className="truncate">{brk ? "Ads" : (a.series ?? a.title)}</span>
            </div>
          );
        })}
        {/* Watched-so-far fill and the playhead. */}
        <div
          className="bg-signal/35 pointer-events-none absolute inset-y-1 left-0 rounded-l"
          style={{ width: `${pct(playhead)}%` }}
        />
        <div
          className="bg-signal absolute -inset-y-0.5 w-0.5 shadow-[0_0_8px_rgb(255_176_32/.8)]"
          style={{ left: `${pct(playhead)}%` }}
        />
        {behind && (
          <div
            className="bg-onair-300 absolute -inset-y-0.5 w-0.5 opacity-70"
            style={{ left: `${pct(NOW)}%` }}
            title="Live"
          />
        )}
      </div>
      <div className="text-static-400 relative mt-1 h-3 font-mono text-[10px]">
        <span className="absolute left-0">−15 min</span>
        <span className="text-onair-300 absolute -translate-x-1/2" style={{ left: `${pct(NOW)}%` }}>
          ● LIVE {clock(NOW)}
        </span>
        <span className="absolute right-0">{clock(NOW + WINDOW_AHEAD)}</span>
      </div>
    </div>
  );
};

const IconButton = ({
  label,
  children,
  active,
}: {
  label: string;
  children: ReactNode;
  active?: boolean;
}) => (
  <button
    type="button"
    aria-label={label}
    title={label}
    className={cn(
      "text-static-100 hover:bg-static-0/12 flex size-9 items-center justify-center rounded-full",
      active && "bg-static-0/15 text-static-0",
    )}
  >
    {children}
  </button>
);

const TopBar = ({ state }: { state: WatchState }) => (
  <div className="absolute inset-x-0 top-0 flex items-start gap-3 bg-linear-to-b from-black/80 to-transparent px-4 pt-3 pb-12">
    <IconButton label="Leave Watch">
      <ArrowLeft className="size-5" />
    </IconButton>
    <div className="min-w-0 flex-1 pt-0.5">
      <div className="flex items-center gap-2">
        <span className="text-static-300 font-mono text-xs">CH {channel.number}</span>
        <span className="text-static-100 text-xs">{channel.name}</span>
      </div>
      <p className="text-static-0 truncate text-base font-semibold">
        {state === "commercials" ? `Commercials · ${commercial.clipTitle}` : programmeLine}
      </p>
    </div>
    <IconButton label="Info (I)" active={state === "info"}>
      <Info className="size-5" />
    </IconButton>
  </div>
);

const Transport = ({ state, touch }: { state: WatchState; touch: boolean }) => {
  const behind = state === "behind";
  return (
    <div className="absolute inset-x-0 bottom-0 flex flex-col gap-2 bg-linear-to-t from-black/85 via-black/50 to-transparent px-4 pt-16 pb-3">
      <Strip behind={behind} />
      <div className="flex items-center gap-1">
        <IconButton label="Back 10 s">
          <RotateCcw className="size-4" />
        </IconButton>
        <button
          type="button"
          aria-label={behind ? "Play" : "Pause"}
          className={cn(
            "bg-static-0 flex items-center justify-center rounded-full text-black",
            touch ? "size-12" : "size-10",
          )}
        >
          {behind ? (
            <Play className="size-5 translate-x-px fill-current" />
          ) : (
            <span className="text-sm font-bold">❙❙</span>
          )}
        </button>
        <IconButton label="Forward 10 s">
          <RotateCw className="size-4" />
        </IconButton>
        {!touch && (
          <div className="ml-1 flex items-center gap-2">
            <Volume2 className="text-static-100 size-4" />
            <div className="bg-static-0/25 h-1 w-20 rounded-full">
              <div className="bg-static-0 h-1 w-3/4 rounded-full" />
            </div>
          </div>
        )}
        <div className="ml-2">
          <LiveIndicator
            state={{
              mode: behind ? "paused" : "live",
              lagSeconds: 134,
              viewerTimeMs: NOW,
              noticeRevision: 0,
            }}
            onGoLive={() => {}}
          />
        </div>
        {behind && (
          <button
            type="button"
            className="border-static-0/20 text-static-100 hover:bg-static-0/10 ml-1 h-7 rounded-full border px-3 text-[11px]"
          >
            ⟲ Start over
          </button>
        )}
        {!touch && (
          <span className="text-static-300 ml-2 font-mono text-xs tabular-nums">
            9:12 <span className="text-static-500">/ 22:00</span> · 13m left
          </span>
        )}
        <div className="ml-auto flex items-center gap-0.5">
          <IconButton label="Channel down">
            <ChevronDown className="size-4" />
          </IconButton>
          <IconButton label="Channel up">
            <ChevronUp className="size-4" />
          </IconButton>
          <button
            type="button"
            className={cn(
              "text-static-100 mx-1 h-8 rounded-lg border px-3 text-xs",
              state === "drawer" ? "border-static-0 bg-static-800" : "border-static-0/15 bg-static-0/6",
            )}
          >
            ☰ Channels
          </button>
          <IconButton label="Subtitles (C)">
            <Captions className="size-5" />
          </IconButton>
          <IconButton label="Settings" active={state === "settings" || state === "playback-info"}>
            <Settings className="size-5" />
          </IconButton>
          <IconButton label="Picture-in-picture (P)">
            <PictureInPicture2 className="size-5" />
          </IconButton>
          {!touch && (
            <IconButton label="Fullscreen (F)">
              <Maximize className="size-4" />
            </IconButton>
          )}
        </div>
      </div>
    </div>
  );
};

const Card = ({ className, children }: { className?: string; children: ReactNode }) => (
  <div
    className={cn(
      "border-static-0/10 bg-static-950/90 text-static-100 rounded-xl border shadow-2xl backdrop-blur-md",
      className,
    )}
  >
    {children}
  </div>
);

const Row = ({ label, value }: { label: string; value: string }) => (
  <div className="hover:bg-static-0/8 flex items-center justify-between gap-6 rounded-md px-3 py-2 text-sm">
    <span>{label}</span>
    <span className="text-static-300">{value} ›</span>
  </div>
);

const SettingsMenu = () => {
  return (
    <Card className="absolute right-4 bottom-24 w-72 p-1.5">
      <Row label="Quality" value={`Auto · ${quality.current}`} />
      <Row label="Audio" value={audio.current} />
      <Row label="Subtitles" value={subtitles.current} />
      <div className="bg-static-0/10 my-1 h-px" />
      <Row label="Playback info" value="" />
      <div className="bg-static-0/10 my-1 h-px" />
      {/* One submenu opened, so the rung list is judged too. */}
      <p className="text-static-400 px-3 pt-1 text-[10px] tracking-widest uppercase">Quality</p>
      {["Auto · 720p", ...quality.rungs].map((r, i) => (
        <div key={r} className="flex items-center gap-2 px-3 py-1.5 text-sm">
          <span className={cn("w-3", i === 0 ? "text-signal" : "invisible")}>✓</span>
          {r}
        </div>
      ))}
    </Card>
  );
};

const InfoSheet = () => (
  <Card className="absolute bottom-28 left-4 flex w-[min(560px,calc(100%-2rem))] gap-4 p-4">
    <div className="from-static-700 to-static-900 aspect-video w-40 shrink-0 rounded-md bg-linear-to-br" />
    <div className="min-w-0">
      <p className="text-static-400 text-xs">
        {current?.series} · S{current?.season} E{current?.episode}
      </p>
      <p className="text-static-0 text-lg font-semibold">{current?.title}</p>
      <p className="text-static-400 mt-0.5 text-xs">
        {current?.year} · {current?.rating} · {current?.genres?.join(", ")} · {clock(current?.startMs ?? 0)}–
        {clock(current?.stopMs ?? 0)}
      </p>
      <p className="text-static-200 mt-2 line-clamp-3 text-sm">{current?.description}</p>
      <div className="bg-static-0/15 mt-3 h-1 rounded-full">
        <div className="bg-signal h-1 w-[41%] rounded-full" />
      </div>
      <p className="text-static-300 mt-3 text-xs">
        <span className="text-static-400 tracking-widest uppercase">Up next</span> ·{" "}
        {clock(next?.startMs ?? 0)} ·{" "}
        <span className="text-static-100">
          {next?.series} — {next?.title}
        </span>
      </p>
    </div>
  </Card>
);

const PlaybackInfo = () => (
  <Card className="absolute top-20 left-4 w-80 p-3 font-mono text-[11px]">
    <p className="text-static-0 mb-2 font-sans text-sm font-semibold">Playback info</p>
    {playbackInfo.viewer.map(([k, v]) => (
      <div key={k} className="flex justify-between gap-3 py-0.5">
        <span className="text-static-400">{k}</span>
        <span>{v}</span>
      </div>
    ))}
    <p className="text-static-400 mt-2 mb-1 font-sans text-[10px] tracking-widest uppercase">
      Server · admin
    </p>
    {playbackInfo.admin.map(([k, v]) => (
      <div key={k} className="flex justify-between gap-3 py-0.5">
        <span className="text-static-400">{k}</span>
        <span className="truncate text-right">{v}</span>
      </div>
    ))}
    <button
      type="button"
      className="border-static-0/15 hover:bg-static-0/8 mt-3 w-full rounded-md border py-1.5 font-sans text-xs"
    >
      Copy stream URL
    </button>
  </Card>
);

const Drawer = () => (
  <div className="border-static-0/10 bg-static-950/92 absolute top-0 right-0 bottom-28 w-80 overflow-y-auto rounded-bl-xl border-b border-l p-3 backdrop-blur-md">
    <p className="text-static-0 mb-2 text-sm font-semibold">Channels</p>
    {channels.map((c) => (
      <div
        key={c.number}
        className={cn(
          "flex items-center gap-3 rounded-lg px-2 py-2",
          c.number === channel.number ? "bg-static-0/10 ring-signal ring-1" : "hover:bg-static-0/6",
        )}
      >
        <span className="text-static-300 w-6 font-mono text-xs">{c.number}</span>
        <span className="min-w-0 flex-1">
          <span className="text-static-100 block truncate text-sm">{c.name}</span>
          <span className="text-static-400 block truncate text-xs">{c.now}</span>
        </span>
        <Star className={cn("size-4", c.favourite ? "fill-signal text-signal" : "text-static-500")} />
      </div>
    ))}
  </div>
);

const HELP: [string, string][] = [
  ["Space / K", "Pause · play"],
  ["← / →", "Back / forward 10 s"],
  ["L", "Go live"],
  ["↑ ↓ · PgUp PgDn", "Channel up / down"],
  ["0–9 · Enter", "Tune by number"],
  ["G", "Channels"],
  ["M", "Mute"],
  ["− / =", "Volume"],
  ["F", "Fullscreen"],
  ["P", "Picture-in-picture"],
  ["C", "Subtitles"],
  ["I", "Info"],
  ["?", "This sheet"],
  ["Esc", "Close · leave"],
];

const Help = () => (
  <div className="absolute inset-0 flex items-center justify-center bg-black/55">
    <Card className="w-[min(520px,calc(100%-2rem))] p-5">
      <p className="text-static-0 mb-3 text-base font-semibold">Keyboard shortcuts</p>
      <div className="grid grid-cols-2 gap-x-6 gap-y-1.5">
        {HELP.map(([k, v]) => (
          <div key={k} className="flex items-center justify-between gap-3 text-sm">
            <span className="text-static-200">{v}</span>
            <kbd className="border-static-0/15 bg-static-0/8 rounded border px-1.5 font-mono text-[11px]">
              {k}
            </kbd>
          </div>
        ))}
      </div>
    </Card>
  </div>
);

const StationId = () => (
  <div className="motion-safe:animate-in motion-safe:slide-in-from-left-4 absolute top-6 left-6 flex items-stretch overflow-hidden rounded-lg shadow-2xl">
    <div className="bg-signal flex w-20 flex-col items-center justify-center text-black">
      <span className="text-[10px] font-bold tracking-widest">CH</span>
      <span className="font-mono text-3xl leading-none font-bold">{channel.number}</span>
    </div>
    <div className="bg-static-950/90 px-4 py-2.5 backdrop-blur-md">
      <p className="text-static-0 text-lg font-semibold">{channel.name}</p>
      <p className="text-static-400 text-[11px] tracking-wider uppercase">
        {channel.era} · {channel.place}
      </p>
      <p className="text-static-200 mt-1 text-sm">{programmeLine}</p>
    </div>
  </div>
);

const Message = ({ title, sub, retry }: { title: string; sub: string; retry?: boolean }) => (
  <div className="from-static-800 absolute inset-0 flex flex-col items-center justify-center gap-2 bg-radial to-black px-8 text-center">
    <p className="text-static-0 text-lg font-semibold">{title}</p>
    <p className="text-static-400 text-sm">{sub}</p>
    {retry && (
      <button
        type="button"
        className="border-static-0/20 text-static-100 mt-2 rounded-md border px-3 py-1.5 text-sm"
      >
        Retry channel
      </button>
    )}
  </div>
);

// The whole overlay player for one state: desktop and phone landscape share it (landscape = touch).
const Overlay = ({ state, touch }: { state: WatchState; touch: boolean }) => {
  if (state === "tuning") return <TunerLoader channel={channel} heldFrame />;
  if (state === "error") return <Message title="The stream stopped." sub="Coast 90s didn't answer." retry />;
  if (state === "off-air")
    return (
      <Message title="Coast 90s is off air" sub="This channel is paused, so there's nothing to watch." />
    );
  if (state === "idle") return null;
  if (state === "station-id") return <StationId />;
  return (
    <>
      <TopBar state={state} />
      <Transport state={state} touch={touch} />
      {state === "settings" && <SettingsMenu />}
      {state === "info" && <InfoSheet />}
      {state === "playback-info" && <PlaybackInfo />}
      {state === "drawer" && <Drawer />}
      {state === "help" && <Help />}
    </>
  );
};

// Portrait phone: N5 kept — the picture clean on top, the panel below gains ⚙ and ⓘ.
const Portrait = ({ state }: { state: WatchState }) => (
  <div className="bg-static-950 flex h-full flex-col">
    <div className="relative aspect-video w-full">
      <PrototypePicture />
      {state === "tuning" && <TunerLoader channel={channel} heldFrame />}
      {state === "station-id" && (
        <div className="bg-signal absolute top-2 left-2 rounded px-2 py-0.5 font-mono text-xs font-bold text-black">
          CH {channel.number}
        </div>
      )}
    </div>
    <div className="flex flex-1 flex-col gap-4 p-4">
      <div className="flex items-center gap-2">
        <span className="text-static-300 font-mono text-xs">CH {channel.number}</span>
        <span className="text-static-100 text-sm font-semibold">{channel.name}</span>
        <Star className="fill-signal text-signal ml-auto size-5" />
      </div>
      <div>
        <p className="text-static-0 font-semibold">
          {state === "commercials" ? `Commercials · ${commercial.clipTitle}` : programmeLine}
        </p>
        <div className="bg-static-0/15 mt-2 h-1 rounded-full">
          <div className="bg-signal h-1 w-[41%] rounded-full" />
        </div>
        <p className="text-static-400 mt-1 text-xs">
          13m left · Up next {clock(next?.startMs ?? 0)} {next?.series}
        </p>
      </div>
      {state === "info" && <p className="text-static-200 text-sm">{current?.description}</p>}
      <div className="grid grid-cols-5 gap-2">
        {["⟲10", state === "behind" ? "▶" : "❙❙", "⟳10", "▲", "▼"].map((l) => (
          <button key={l} type="button" className="bg-static-0/8 text-static-100 h-12 rounded-xl text-sm">
            {l}
          </button>
        ))}
      </div>
      <div className="flex gap-2">
        <button type="button" className="bg-static-0/8 text-static-100 h-11 flex-1 rounded-xl text-sm">
          <Info className="mr-1 inline size-4 align-[-3px]" />
          Info
        </button>
        <button type="button" className="bg-static-0/8 text-static-100 h-11 flex-1 rounded-xl text-sm">
          ⚙ Settings
        </button>
        <button type="button" className="bg-static-0/8 text-static-100 h-11 flex-1 rounded-xl text-sm">
          ⧉ PiP
        </button>
      </div>
      {state === "settings" && (
        <Card className="p-2 text-sm">
          <div className="flex justify-between p-2">
            <span>Quality</span>
            <span className="text-static-300">Auto · 720p ›</span>
          </div>
          <div className="flex justify-between p-2">
            <span>Audio</span>
            <span className="text-static-300">{audio.current} ›</span>
          </div>
          <div className="flex justify-between p-2">
            <span>Subtitles</span>
            <span className="text-static-300">{subtitles.current} ›</span>
          </div>
        </Card>
      )}
      <div>
        <p className="text-static-400 mb-2 text-[10px] tracking-widest uppercase">Favorites</p>
        {channels
          .filter((c) => c.favourite)
          .map((c) => (
            <div key={c.number} className="flex gap-3 py-1.5 text-sm">
              <span className="text-static-400 w-6 font-mono">{c.number}</span>
              <span className="text-static-100">{c.name}</span>
              <span className="text-static-400 ml-auto">{c.now}</span>
            </div>
          ))}
      </div>
    </div>
  </div>
);

const VariantA = ({ state, device }: VariantProps) =>
  device === "portrait" ? (
    <Portrait state={state} />
  ) : (
    <div className="absolute inset-0">
      <PrototypePicture />
      <Overlay state={state} touch={device === "landscape"} />
    </div>
  );

export { VariantA };
