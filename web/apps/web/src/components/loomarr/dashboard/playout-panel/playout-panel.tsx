import type { ChannelHealth, PlayoutGPU, PreparedReadiness } from "@loomarr/api";
import { formatBytes, formatRelative, pluralize } from "@loomarr/core";
import { Info } from "lucide-react";
import { Badge, Card, PanelRow, Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui";
import { cn } from "@/lib";
import type { PlayoutPanelProps } from "./playout-panel.type";

// PlayoutPanel — the dashboard's single live-playout picture (§12, §9.1 V47, GET /v1/playout/status).
//
// It MERGES what used to be two overlapping panels (TranscodingPanel + PlayoutHealthPanel), which
// repeated channel id, speed and hardware per row. One panel now: a GPU/LLM-contention header — the
// biggest silent cause of stutter, an LLM sat resident on the GPU the encoders need (§8.2) — and one
// row per channel carrying everything non-duplicated: identity, throughput, cold-start, verdict.

// health/target/mode/encoder are free strings from the backend (§ no-hardcode) — never a closed enum
// on the frontend. Only the Badge TONE is a closed mapping, and it degrades safely: an unrecognised
// health string still renders, just untinted.
const HEALTH_TONE: Record<string, "lock" | "caution"> = {
  ok: "lock",
  degraded: "caution",
  stalled: "caution",
};

// "—" for a channel with no speed sample yet (direct-play copies report 0), never "0.0× rt", which
// reads as stalled when it means "no sample".
const formatSpeed = (speed: number): string => (speed > 0 ? `${speed.toFixed(1)}× rt` : "—");

const formatVram = (gib: number): string => `${gib.toFixed(1)} GB`;

// formatBuffer: how far ahead of realtime the encoder has produced output; negative = behind.
const formatBuffer = (ms: number): string => {
  const secs = Math.round(ms / 1000);
  return secs < 0 ? `${Math.abs(secs)}s behind` : `${secs}s ahead`;
};

// formatColdStart: time-to-first-frame — the black-screen window a viewer waited through. "—" when
// unmeasured (0), else seconds with one decimal ("1.3s to play").
const formatColdStart = (ms: number): string => (ms > 0 ? `${(ms / 1000).toFixed(1)}s to play` : "—");

// channelLabel prefers the human identity "#3 · 1980s Action Heroes"; falls back to the raw id when
// the channel is not in the store (a just-deleted channel still finishing its stream).
const channelLabel = (c: ChannelHealth): string => (c.name ? `#${c.number} · ${c.name}` : c.channelId);

// GpuRow is the shared-GPU CONTEXT HEADER for the channel rows below it — not another row. It gets a
// tinted band + a mono "GPU" eyebrow so the eye parses it as the header it is, distinct from the
// per-channel list.
const GpuRow = ({ gpu }: { gpu: PlayoutGPU }) => {
  if (!gpu.vramGiB) {
    return (
      <div className="border-border border-b bg-static-900/40 px-4 py-2">
        <p className="font-mono text-2xs text-muted-foreground uppercase tracking-wide">GPU</p>
        <p className="mt-0.5 text-muted-foreground text-sm">Software encoding — no GPU on this host</p>
      </div>
    );
  }
  return (
    <div className="flex items-center justify-between gap-3 border-border border-b bg-static-900/40 px-4 py-2.5">
      <div className="min-w-0">
        <p className="font-mono text-2xs text-muted-foreground uppercase tracking-wide">GPU</p>
        <p className="mt-0.5 flex flex-wrap items-baseline gap-x-2 gap-y-0.5">
          <span className="truncate font-medium text-sm">{gpu.name ?? "GPU"}</span>
          <span className="text-muted-foreground text-xs">{formatVram(gpu.vramGiB)} total</span>
          {gpu.llmModel && (
            <span className="text-muted-foreground text-xs">
              · {gpu.llmModel} holding {formatVram(gpu.llmVramGiB ?? 0)}
            </span>
          )}
        </p>
      </div>
      {/* The whole reason the GPU header exists: a resident LLM sharing VRAM with the encoders is a
          real, silent cause of stutter, and nothing else on the dashboard says so. */}
      {gpu.contended && <Badge variant="caution">LLM sharing VRAM</Badge>}
    </div>
  );
};

const PreparedRow = ({ prepared }: { prepared?: PreparedReadiness }) => {
  if (!prepared?.available) {
    return (
      <div className="border-border border-b px-4 py-3">
        <div className="flex items-center justify-between gap-3">
          <p className="font-medium text-sm">Prepared playback unavailable</p>
          <Badge variant="caution">Live fallback only</Badge>
        </div>
        <p className="mt-1 text-muted-foreground text-xs">
          {prepared?.unavailableReason ?? "The readiness planner is not wired."}
        </p>
      </div>
    );
  }

  if (!prepared.lastRunAt) {
    return (
      <div className="border-border border-b px-4 py-3">
        <div className="flex items-center justify-between gap-3">
          <p className="font-medium text-sm">Prepared playback</p>
          {prepared.running && <Badge variant="neutral">Warming</Badge>}
        </div>
        <p className="mt-1 text-muted-foreground text-xs">
          {prepared.running ? "The first readiness pass is running…" : "Readiness pass hasn’t run yet."}
        </p>
      </div>
    );
  }

  const allReady = prepared.readyChannels === prepared.channels;
  return (
    <div className="border-border border-b px-4 py-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <p className="font-mono text-2xs text-muted-foreground uppercase tracking-wide">
            Prepared playback
          </p>
          <p className="mt-0.5 font-medium text-sm">
            {prepared.channels === 0
              ? "No scheduled channels in this window"
              : `${prepared.readyChannels} of ${prepared.channels} channels ready`}
          </p>
        </div>
        <div className="flex items-center gap-2">
          {prepared.running && <Badge variant="neutral">Pass running</Badge>}
          <Badge variant={allReady ? "lock" : "caution"}>{allReady ? "Ready" : "Warming"}</Badge>
        </div>
      </div>
      <p className="mt-1.5 text-muted-foreground text-xs">
        {pluralize(prepared.missingBindings, "scheduled binding")} unprepared ·{" "}
        {pluralize(prepared.queuedPublications, "publication")} warming
      </p>
      <p className="mt-1 text-muted-foreground text-xs">
        {formatBytes(prepared.remainingBytes)} of {formatBytes(prepared.budgetBytes)} used ·{" "}
        {formatBytes(prepared.protectedBytes)} protected · updated {formatRelative(prepared.lastRunAt)}
      </p>
      {prepared.lastError && <p className="mt-1 text-caution text-xs">Last pass: {prepared.lastError}</p>}
    </div>
  );
};

const ChannelRow = ({ channel }: { channel: ChannelHealth }) => {
  const slow = channel.speed > 0 && channel.speed < 1;
  return (
    <PanelRow>
      <PanelRow.Main>
        <div className="flex items-center gap-1.5">
          <p className="truncate font-medium text-sm">{channelLabel(channel)}</p>
          {/* The raw id lives behind an (i) affordance — noise on the row, useful for a support
              request. The app-wide Tooltip (TooltipProvider at __root.tsx) gives an instant, styled
              reveal on hover/focus, same idiom as FieldHelp; the aria-label keeps it screen-reader
              reachable. (A native `title` also "worked" but its ~1.5s browser delay read as broken.) */}
          <Tooltip>
            <TooltipTrigger
              render={
                <button
                  type="button"
                  aria-label={`Channel id ${channel.channelId}`}
                  className="inline-flex size-4 shrink-0 cursor-help items-center justify-center rounded-full text-static-400 transition-colors hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                />
              }
            >
              <Info className="size-3" aria-hidden />
            </TooltipTrigger>
            <TooltipContent>
              <span className="font-mono text-xs">{channel.channelId}</span>
            </TooltipContent>
          </Tooltip>
        </div>
        <p className="mt-0.5 truncate text-muted-foreground text-xs">
          {`${channel.target} · ${channel.mode}${channel.mode === "transcode" ? ` · ${channel.encoder}` : ""} · ${pluralize(channel.viewers, "viewer")}`}
        </p>
      </PanelRow.Main>

      <PanelRow.Meta>
        <div className="text-right">
          <p className="font-mono text-static-200 text-xs">{formatColdStart(channel.coldStartMs)}</p>
          <p className="mt-0.5 font-mono text-2xs text-muted-foreground">
            {formatBuffer(channel.bufferedMs)}
          </p>
        </div>
        <span
          className={cn(
            "w-14 text-right font-mono text-xs tabular-nums",
            slow ? "text-caution" : "text-muted-foreground",
          )}
        >
          {formatSpeed(channel.speed)}
        </span>
        {/* A direct-play channel is REMUXED, not encoded — so "Hardware/Software" (an encoder
            distinction) is meaningless for it; it reads "Copy". Only a transcode has an encoder. */}
        {channel.mode === "direct-play" ? (
          <Badge variant="neutral">Copy</Badge>
        ) : (
          <Badge variant={channel.hardware ? "lock" : "caution"}>
            {channel.hardware ? "Hardware" : "Software"}
          </Badge>
        )}
        <Badge variant={HEALTH_TONE[channel.health] ?? "neutral"}>{channel.health}</Badge>
      </PanelRow.Meta>
    </PanelRow>
  );
};

const PlayoutPanel = ({ status, loading, title = "Playout", className }: PlayoutPanelProps) => {
  const channels = status?.channels ?? [];

  return (
    <Card className={cn("flex shrink-0 flex-col overflow-hidden p-0", className)}>
      <div className="flex items-center justify-between gap-3 border-border border-b px-4 py-3">
        <h2 className="font-semibold text-base">{title}</h2>
        {!loading && status != null && (
          <Badge variant="neutral">{status.running ? "Loomarr" : "Tunarr"}</Badge>
        )}
      </div>

      {loading && <p className="px-4 py-6 text-muted-foreground text-sm">Reading playout status…</p>}

      {/* On a Tunarr-backed install there is no internal playout to show, which is not the same as
          every channel being unhealthy. */}
      {!loading && status != null && !status.running && (
        <p className="px-4 py-6 text-muted-foreground text-sm">
          Loomarr isn't streaming these channels. Tunarr is.
        </p>
      )}

      {!loading && status?.running && <GpuRow gpu={status.gpu} />}

      {!loading && status?.running && <PreparedRow prepared={status.prepared} />}

      {!loading && status?.running && channels.length === 0 && (
        <p className="px-4 py-6 text-muted-foreground text-sm">
          No live fallback encoders are active. Prepared viewers do not create a row here.
        </p>
      )}

      {status?.running && channels.length > 0 && (
        <ul className="flex flex-col">
          {channels.map((c) => (
            <ChannelRow key={`${c.channelId}:${c.target}`} channel={c} />
          ))}
        </ul>
      )}
    </Card>
  );
};

export { PlayoutPanel };
