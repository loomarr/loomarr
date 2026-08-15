import * as channelsApi from "@loomarr/api/endpoints/channels";
import type { ChannelDTO } from "@loomarr/api/models/channelDTO";
import type { ChannelPolicy } from "@loomarr/api/models/channelPolicy";
import type { GuideAiring } from "@loomarr/api/models/guideAiring";
import type { TrackDTO } from "@loomarr/api/models/trackDTO";
import { unwrap } from "@loomarr/api/unwrap";
import { ChevronDown, ChevronUp, Play, Volume2 } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";
import { useHlsPlayer } from "@/channels/use-hls-player";
import { TunerLoader } from "@/components/loomarr/shell/tuner-loader";
import { Button } from "@/components/ui/button";
import { VideoPlayer } from "@/components/ui/video-player";
import { TimelineScrubber } from "@/components/ui/video-player/timeline-scrubber";
import { TrackSelectMenu } from "@/components/ui/video-player/track-select-menu";
import { TunerOSD } from "../tuner-osd";
import type { TuneAttempt } from "../tuner-timing";
import type { TuneDirection } from "../use-channel-tuner";
import { languageLabel } from "./language-label";

// ChannelWatch — the Watch sub-section: play a channel live in the browser (§9.1, V46).
//
// The player itself is the shared VideoPlayer primitive in `live` mode (no video seek — a live
// channel has nothing to seek to) with hls.js bound through its `attach` seam (useHlsPlayer). In
// place of a seek bar it passes VideoPlayer a `scrubber`: the mini-guide timeline (ChannelTimeline,
// §9.1 V47), so the control bar shows where you are in the schedule. This component owns the
// surrounding SURFACE: the idle "▶ Watch live" poster, the full-frame theater, the channel controls.
//
// ⚠ Audio is CHANNEL-WIDE and admin-only, not per-viewer (§9.1). Internal playout
// is one encoder per channel fanned to every viewer, so a per-viewer track would fork the encode —
// the one thing that model forbids. So these pickers write `policy.playout.*` for the whole
// channel; a member sees the current values read-only. Quality is the ONLY per-viewer control, and
// it is handled inside the player (the client adapts live), not here.

interface ChannelWatchProps {
  channel: ChannelDTO;
  isAdmin: boolean;
  /** Save a whole policy (channel-level audio override). */
  onSavePolicy: (policy: ChannelPolicy) => void;
  /** Media-server name for the "Open in …" hand-off; defaults to "your media server". */
  mediaServerName?: string;
  tuner?: {
    canSurf: boolean;
    currentTitle?: string;
    attempt?: TuneAttempt;
    step: (direction: TuneDirection) => void;
    retry: () => void;
  };
}

// withSaved keeps the currently-saved value present in an options list even when the airing does
// not carry that track — so the menu always shows the channel's real selection (see the call site).
const withSaved = (options: { value: string; label: string }[], value: string) =>
  value && !options.some((o) => o.value === value)
    ? [...options, { value, label: value.toUpperCase() }]
    : options;

// AUTO_SENTINEL is the "follow the channel/global default" choice. Radix Select forbids an
// empty-string item value, so the picker carries this named sentinel and lowers it back to "" on
// the wire — the same shape channel-seasonal/ordering use for their "" defaults.
const AUTO_SENTINEL = "auto";

// audioOptions builds the Audio picker's choices from the REAL audio tracks of what's airing
// (§9.1, V46) — never a hardcoded list. Always leads with "Auto", then one entry per distinct track
// language the airing media carries. Labels come from Intl (languageLabel), not a hand-written
// language table.
const audioOptions = (tracks: TrackDTO[]): { value: string; label: string }[] => {
  const seen = new Set<string>();
  const opts: { value: string; label: string }[] = [{ value: AUTO_SENTINEL, label: "Auto (default)" }];
  for (const t of tracks) {
    const lang = t.language ?? "";
    if (!lang || seen.has(lang)) continue;
    seen.add(lang);
    opts.push({ value: lang, label: t.title ? `${languageLabel(lang)} · ${t.title}` : languageLabel(lang) });
  }
  return opts;
};

// clock renders ms-into-a-programme as m:ss (the mock's {elapsed} / {total} format).
const clock = (ms: number): string => {
  const total = Math.max(0, Math.floor(ms / 1000));
  return `${Math.floor(total / 60)}:${String(total % 60).padStart(2, "0")}`;
};

// programmeTime renders the controls-row time for the programme airing NOW (the mock's
// "{elapsed} / {total}   {remaining}"): how far into the current show, the show's length, and the
// time left — all PROGRAMME time from the schedule, not video time. null when nothing is airing at
// this instant (the player omits the time). Rendered as a node so the "/ total" is dimmed like the
// mock and the "left" reads grey beside it.
const programmeTime = (airings: GuideAiring[]): React.ReactNode => {
  const now = Date.now();
  const current = airings.find((a) => now >= a.startMs && now < a.stopMs);
  if (!current) return null;
  const elapsed = now - current.startMs;
  const total = current.stopMs - current.startMs;
  const leftMin = Math.max(0, Math.round((current.stopMs - now) / 60_000));
  return (
    <span className="flex items-center gap-2">
      <span>
        {clock(elapsed)} <span className="text-muted-foreground">/ {clock(total)}</span>
      </span>
      <span className="text-muted-foreground">{leftMin}m left</span>
    </span>
  );
};

const ChannelWatch = ({
  channel,
  isAdmin,
  onSavePolicy,
  mediaServerName = "your media server",
  tuner,
}: ChannelWatchProps) => {
  const player = useHlsPlayer(channel.id, tuner?.attempt);
  // `active` gates the idle poster vs the live player, and it now starts TRUE: opening Watch tunes
  // in (§9.1 V54). Watch is the first section a channel opens on, and a player that sits behind a
  // second click makes "open the channel" a two-step act to do the obvious thing.
  //
  // ⚠ **This does NOT guarantee the video starts, and that is a browser rule, not a bug.** Autoplay
  // with sound requires user activation. Reaching here by CLICKING a channel keeps the document's
  // sticky activation, so it plays; arriving by a pasted link or a reload has no activation and the
  // browser will hold the first frame with the controls showing. Mounting the player either way is
  // still the right call — a paused player with a play button is one click from playing, which is
  // exactly where the old poster left you, and every other visit skips the click entirely.
  //
  // The poster is kept for the paused/off-air case below, which is a different claim: nothing to
  // play at all, rather than something waiting for permission to start.
  const [active, setActive] = useState(true);

  const paused = channel.status === "paused" || channel.status === "detached";

  // The pickers' options are the tracks the airing programme actually carries — fetched, never
  // hardcoded. `enabled` gates the probe on a playing channel (a paused one has nothing to probe).
  const tracks = channelsApi.useChannelTracks(channel.id, { query: { enabled: !paused, retry: false } });
  const tracksBody = unwrap(tracks.data);

  // The mini-guide scrubber's data — the channel's schedule strip (now + next few + the commercial
  // breaks between them, each with episode detail + a TMDB still). Only while active (a poster needs
  // no timeline) and unpaused. Refetched on an interval so the live playhead and "what's next" stay
  // current as programmes roll.
  const timeline = channelsApi.useChannelTimeline(channel.id, undefined, {
    query: { enabled: active && !paused, retry: false, refetchInterval: 30_000 },
  });
  const airings = unwrap(timeline.data)?.airings ?? [];
  const audioValue = channel.policy?.playout?.audioLanguage ?? "";

  // The channel may be set to a track the CURRENTLY-airing programme doesn't carry (set to French,
  // but this film is English-only). Keep that selection VISIBLE in the menu rather than dropping it —
  // the preference still applies when a programme with that track airs. So the saved value is
  // appended to the media-derived options when absent, labelled by its raw code. (Ported from the
  // old footer PolicyPicker.)
  const audioOpts = withSaved(audioOptions(tracksBody?.audio ?? []), audioValue || AUTO_SENTINEL);

  const savePlayout = (patch: { audioLanguage?: string }) => {
    const next: ChannelPolicy = {
      ...channel.policy,
      playout: { ...channel.policy?.playout, ...patch },
    };
    onSavePolicy(next);
    toast.success("Channel updated — applies on the next segment.");
  };

  const openInMediaServer = () =>
    toast.info(`Opening ${channel.name} in ${mediaServerName} — same stream, your usual client.`);

  // The mini-guide scrubber (§9.1 V47) fills the player's full-width `scrubber` slot. Shown once we
  // have a timeline and the stream is healthy; otherwise the player's control bar has no scrubber row.
  const scrubber =
    airings.length > 0 && player.status !== "error" ? <TimelineScrubber airings={airings} /> : undefined;

  // The controls-row time (mock): elapsed / total + "N min left" for the programme airing now, from
  // the schedule (the player has no source for programme time, so channel-watch derives it).
  const timeLeft = programmeTime(airings);

  // The player's live top bar: "CH {n}" (left, after the LIVE badge) + the channel name, matching the
  // mock's "CH 3" line. The encoder line ("h264 · 1080p") the mock also shows is admin telemetry not
  // fetched here; the channel identity is what a viewer needs.
  const topBar = (
    <>
      <span className="shrink-0 font-mono text-static-300 text-xs">CH {channel.number}</span>
      <span className="min-w-0 truncate text-static-200 text-xs">{channel.name}</span>
    </>
  );

  // Audio control IN the player bar (§9.1 V47), beside fullscreen — the maintainer's
  // move off the old footer pickers. Same channel-wide, admin-scoped semantics: a member sees the
  // current track (readOnly) but cannot change it. Options are the airing's real tracks (fetched).
  const barControls = (
    <>
      {tuner && (
        <fieldset className="flex min-w-0 items-center gap-0.5 border-0 p-0">
          <legend className="sr-only">Channel navigation</legend>
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className="size-8 text-static-200 hover:bg-static-700 hover:text-static-50"
            aria-label="Channel down"
            disabled={!tuner.canSurf}
            onClick={() => tuner.step(-1)}
          >
            <ChevronDown className="size-4" aria-hidden />
          </Button>
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className="size-8 text-static-200 hover:bg-static-700 hover:text-static-50"
            aria-label="Channel up"
            disabled={!tuner.canSurf}
            onClick={() => tuner.step(1)}
          >
            <ChevronUp className="size-4" aria-hidden />
          </Button>
        </fieldset>
      )}
      <TrackSelectMenu
        icon={Volume2}
        label="Audio"
        options={audioOpts}
        value={audioValue || AUTO_SENTINEL}
        onChange={(v) => savePlayout({ audioLanguage: v === AUTO_SENTINEL ? "" : v })}
        readOnly={!isAdmin}
      />
    </>
  );

  // The player. `live` = no seek; `scrubber` = full-width mini-guide; `topBar`/`timeLeft` = the live
  // chrome; `barControls` = the audio menu beside fullscreen; `attach` binds hls.js. Fullscreen
  // is the player's OWN control. Rendered only when active so the stream is not requested until asked.
  const playerEl = (
    <VideoPlayer
      live
      scrubber={scrubber}
      topBar={topBar}
      timeLeft={timeLeft}
      barControls={barControls}
      // The tuner "acquiring signal" overlay covers the warm-up beat (cold encoder, first segment
      // not cut yet) so the viewer sees a channel tuning in, not a black frame. Only while loading —
      // an error shows its own message below, and a playing stream needs no overlay.
      overlay={player.status === "loading" ? <TunerLoader /> : undefined}
      attach={player.attach}
      onChannelStep={tuner?.step}
      className="overflow-hidden rounded-xl border border-border bg-black"
    />
  );

  return (
    <div className="flex flex-col gap-4">
      <section className="overflow-hidden rounded-xl border border-border bg-card">
        {paused ? (
          <IdleFrame
            title={`${channel.name} is off air`}
            sub="Paused channels broadcast nothing, so there is no stream to join."
          />
        ) : active ? (
          <div className="flex flex-col gap-3 p-3">
            <div className="relative">
              {playerEl}
              {player.status === "loading" && tuner && (
                <TunerOSD
                  number={channel.number}
                  name={channel.name}
                  currentTitle={tuner.currentTitle}
                  className="absolute top-4 left-4 z-[2]"
                />
              )}
            </div>

            {/* The tune-in status line under the player — the player's own control bar carries the
                LIVE badge, scrubber, controls and fullscreen, so this is just the join note. */}
            <p className="px-1 text-muted-foreground text-xs">
              {player.status === "error"
                ? (player.error ?? "The stream stopped.")
                : player.status === "loading"
                  ? "Tuning in…"
                  : "You're joining live, mid-programme."}
            </p>
            {player.status === "error" && tuner && (
              <Button variant="outline" size="sm" onClick={tuner.retry} className="self-start">
                Retry channel
              </Button>
            )}
          </div>
        ) : (
          <button
            type="button"
            onClick={() => setActive(true)}
            className="group relative flex aspect-video w-full cursor-pointer flex-col items-center justify-center gap-3 bg-radial from-static-800 to-black text-center"
            aria-label={`Watch ${channel.name} live`}
          >
            <span className="flex size-14 items-center justify-center rounded-full bg-signal/90 text-black shadow-lg transition-transform group-hover:scale-105">
              <Play className="size-6 translate-x-0.5 fill-current" aria-hidden />
            </span>
            <span className="flex flex-col gap-1">
              <span className="font-semibold text-lg">Watch live</span>
              <span className="text-muted-foreground text-sm">Join {channel.name} mid-programme.</span>
            </span>
          </button>
        )}

        {/* Footer: the "open in your media server" hand-off. Audio lives in the player's
            control bar (§9.1 V47); an admin note explains the channel-wide scoping. */}
        <div className="flex items-center justify-between gap-4 border-border border-t bg-static-900/40 p-4">
          {isAdmin ? (
            <p className="text-muted-foreground text-xs">
              Audio is set for the whole channel — everyone watching hears the same track, because one encoder
              serves them all.
            </p>
          ) : (
            <span />
          )}
          <Button variant="outline" size="sm" onClick={openInMediaServer} className="shrink-0">
            Open in {mediaServerName}
          </Button>
        </div>
      </section>
    </div>
  );
};

// IdleFrame is the poster shown when there is nothing to play (paused/off air).
const IdleFrame = ({ title, sub }: { title: string; sub: string }) => (
  <div className="flex aspect-video w-full flex-col items-center justify-center gap-2 bg-radial from-static-800 to-black px-12 text-center">
    <p className="font-semibold text-lg">{title}</p>
    <p className="text-muted-foreground text-sm">{sub}</p>
  </div>
);

export type { ChannelWatchProps };
export { ChannelWatch };
