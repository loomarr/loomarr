import { type ChannelDTO, type ChannelPolicy, channelsApi, type TrackDTO, unwrap } from "@loomarr/api";
import { Maximize2, Play, X } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";
import { useHlsPlayer } from "@/channels/use-hls-player";
import {
  Button,
  Dialog,
  DialogContent,
  Label,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  VideoPlayer,
} from "@/components/ui";
import { cn } from "@/lib";
import { languageLabel } from "./language-label";

// ChannelWatch — the Watch sub-section: play a channel live in the browser (§9.1, V46).
//
// The player itself is the shared VideoPlayer primitive in `live` mode (no scrubber — a live
// channel has nothing to seek to) with hls.js bound through its `attach` seam (useHlsPlayer). This
// component owns the surrounding SURFACE: the idle "▶ Watch live" poster, the full-frame theater,
// and the channel-level controls.
//
// ⚠ Audio and Subtitles are CHANNEL-WIDE and admin-only, not per-viewer (§9.1). Internal playout
// is one encoder per channel fanned to every viewer, so a per-viewer track would fork the encode —
// the one thing that model forbids. So these pickers write `policy.playout.*` for the whole
// channel; a member sees the current values read-only. Quality is the ONLY per-viewer control, and
// it is handled inside the player (the client adapts live), not here.

interface ChannelWatchProps {
  channel: ChannelDTO;
  isAdmin: boolean;
  /** Save a whole policy (channel-level audio/subtitle overrides). */
  onSavePolicy: (policy: ChannelPolicy) => void;
  /** Media-server name for the "Open in …" hand-off; defaults to "your media server". */
  mediaServerName?: string;
}

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

// subtitleOptions builds the Subtitle picker's choices. The policy is a MODE (off | burn) — burn-in
// uses the preferred-language subtitle track (§9.1) — so the picker offers Off always, and Burn in
// only when the airing media actually HAS a subtitle track to burn. Offering "Burn in" for media
// with no subtitles would be a control that does nothing.
const subtitleOptions = (tracks: TrackDTO[]): { value: string; label: string }[] => {
  const opts = [{ value: "off", label: "Off" }];
  if (tracks.length > 0) {
    const langs = [...new Set(tracks.map((t) => t.language).filter(Boolean))].map((l) =>
      languageLabel(l ?? ""),
    );
    const detail = langs.length > 0 ? ` (${langs.join(", ")})` : "";
    opts.push({ value: "burn", label: `Burn in${detail}` });
  }
  return opts;
};

const ChannelWatch = ({
  channel,
  isAdmin,
  onSavePolicy,
  mediaServerName = "your media server",
}: ChannelWatchProps) => {
  const player = useHlsPlayer(channel.id);
  // `active` gates the idle poster vs the live player. Starting on a click (not on mount) matches
  // the mock's "▶ Watch live" affordance AND satisfies autoplay policies, which need a gesture.
  const [active, setActive] = useState(false);
  const [theater, setTheater] = useState(false);

  const paused = channel.status === "paused" || channel.status === "detached";

  // The pickers' options are the tracks the airing programme actually carries — fetched, never
  // hardcoded. `enabled` gates the probe on a playing channel (a paused one has nothing to probe).
  const tracks = channelsApi.useChannelTracks(channel.id, { query: { enabled: !paused, retry: false } });
  const tracksBody = unwrap(tracks.data);
  const audioOpts = audioOptions(tracksBody?.audio ?? []);
  const subtitleOpts = subtitleOptions(tracksBody?.subtitles ?? []);

  const audioValue = channel.policy?.playout?.audioLanguage ?? "";
  const subtitleValue = channel.policy?.playout?.subtitles || "off";

  const savePlayout = (patch: { audioLanguage?: string; subtitles?: string }) => {
    const next: ChannelPolicy = {
      ...channel.policy,
      playout: { ...channel.policy?.playout, ...patch },
    };
    onSavePolicy(next);
    toast.success("Channel updated — applies on the next segment.");
  };

  const openInMediaServer = () =>
    toast.info(`Opening ${channel.name} in ${mediaServerName} — same stream, your usual client.`);

  // The player element, reused inline and in the theater. `live` hides the scrubber; `attach`
  // binds hls.js. Rendered only when active so the stream is not requested until asked for.
  const playerEl = (
    <VideoPlayer
      live
      attach={player.attach}
      title={`CH ${channel.number} · ${channel.name}`}
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
            {playerEl}
            <div className="flex items-center justify-between gap-2 px-1">
              <span className="text-muted-foreground text-xs">
                {player.status === "error"
                  ? (player.error ?? "The stream stopped.")
                  : player.status === "loading"
                    ? "Tuning in…"
                    : "You're joining live, mid-programme."}
              </span>
              <Button
                variant="outline"
                size="sm"
                onClick={() => setTheater(true)}
                className="shrink-0"
                disabled={player.status === "error"}
              >
                <Maximize2 className="size-3.5" aria-hidden />
                Full frame
              </Button>
            </div>
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

        {/* Controls row: hand-offs everyone sees, plus channel-level audio/subtitle pickers an
            admin can change. */}
        <div className="flex flex-col gap-4 border-border border-t bg-static-900/40 p-4">
          <div className="flex flex-wrap items-end gap-4">
            <PolicyPicker
              label="Audio"
              value={audioValue || AUTO_SENTINEL}
              options={audioOpts}
              isAdmin={isAdmin}
              onChange={(v) => savePlayout({ audioLanguage: v === AUTO_SENTINEL ? "" : v })}
            />
            <PolicyPicker
              label="Subtitles"
              value={subtitleValue}
              options={subtitleOpts}
              isAdmin={isAdmin}
              onChange={(v) => savePlayout({ subtitles: v })}
            />
            <div className="ml-auto flex gap-2">
              <Button variant="outline" size="sm" onClick={openInMediaServer}>
                Open in {mediaServerName}
              </Button>
            </div>
          </div>
          {isAdmin && (
            <p className="text-muted-foreground text-xs">
              Audio and subtitles are set for the whole channel — everyone watching sees the same, because one
              encoder serves them all.
            </p>
          )}
        </div>
      </section>

      {/* Theater: the same live player, full frame. Reuses the Dialog primitive; the player is
          re-mounted here (its own attach), so closing the theater tears that instance down while
          the inline one keeps running. */}
      <Dialog open={theater} onOpenChange={setTheater}>
        <DialogContent
          className="max-w-[min(1100px,95vw)] border-none bg-transparent p-0 shadow-none"
          aria-label={`${channel.name}, full frame`}
        >
          <VideoPlayer
            live
            autoPlay
            attach={player.attach}
            title={`CH ${channel.number} · ${channel.name}`}
            leading={
              <Button
                variant="ghost"
                size="icon"
                onClick={() => setTheater(false)}
                aria-label="Close full frame"
                className="size-8 rounded-full bg-black/40 text-static-100 hover:bg-black/60"
              >
                <X className="size-4" aria-hidden />
              </Button>
            }
            className="overflow-hidden rounded-xl border border-border bg-black"
          />
        </DialogContent>
      </Dialog>
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

// PolicyPicker is a channel-level select: an editable dropdown for an admin, a read-only value for
// a member (who sees what the channel is set to but cannot change it).
const PolicyPicker = ({
  label,
  value,
  options,
  isAdmin,
  onChange,
}: {
  label: string;
  value: string;
  options: { value: string; label: string }[];
  isAdmin: boolean;
  onChange: (v: string) => void;
}) => {
  // The channel may be set to a track the CURRENTLY-airing programme doesn't carry (set to French,
  // but this film is English-only). Keep that selection visible rather than dropping it — the
  // preference still applies when a programme with that track airs. So the saved value is appended
  // to the media-derived options when absent, labelled as its raw code.
  const opts =
    value && !options.some((o) => o.value === value)
      ? [...options, { value, label: value.toUpperCase() }]
      : options;
  const current = opts.find((o) => o.value === value) ?? opts[0];
  const id = `watch-${label.toLowerCase()}`;
  return (
    <div className="flex flex-col gap-1.5">
      <Label
        htmlFor={id}
        className={cn("font-mono text-[10px] text-muted-foreground uppercase tracking-wide")}
      >
        {label}
      </Label>
      {isAdmin ? (
        <Select value={value} onValueChange={onChange}>
          <SelectTrigger id={id} className="h-9 w-44">
            <SelectValue placeholder={current?.label} />
          </SelectTrigger>
          <SelectContent>
            {opts.map((o) => (
              <SelectItem key={o.value} value={o.value}>
                {o.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      ) : (
        <span id={id} className="flex h-9 items-center text-sm">
          {current?.label}
        </span>
      )}
    </div>
  );
};

export type { ChannelWatchProps };
export { ChannelWatch };
