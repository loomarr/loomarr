import type { ChannelHealth } from "@loomarr/api/models/channelHealth";
import type { PlayoutCapability } from "@loomarr/api/models/playoutCapability";
import type { PlayoutGPU } from "@loomarr/api/models/playoutGPU";
import { pluralize } from "@loomarr/core/format";
import { Action, Badge, type BadgeTone, Card, Stack, Text, Tooltip } from "@loomarr/design-system";
import { ListGroup, PanelRow } from "@loomarr/ui";
import type { PlayoutPanelProps } from "./playout-panel.type";

// PlayoutPanel — the dashboard's single live-playout picture (§12, §9.1 V47, GET /v1/playout/status).
//
// ONE panel for transcoding and playout health, so channel id, speed and hardware are not repeated
// across two overlapping lists: a GPU/LLM-contention header — the
// biggest silent cause of stutter, an LLM sat resident on the GPU the encoders need (§8.2) — and one
// row per channel carrying everything non-duplicated: identity, throughput, cold-start, verdict.

// health/target/mode/encoder are free strings from the backend (§ no-hardcode) — never a closed enum
// on the frontend. Only the Badge TONE is a closed mapping, and it degrades safely: an unrecognised
// health string still renders, just untinted.
const HEALTH_TONE: Record<string, BadgeTone> = {
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

// EncoderRow leads with the capability Loomarr actually proved. GPU/VRAM is optional context:
// Intel and AMD hardware encoders work through /dev/dri without NVIDIA-style VRAM telemetry.
const EncoderRow = ({ capability, gpu }: { capability: PlayoutCapability; gpu: PlayoutGPU }) => {
  const capacity = pluralize(capability.maxChannels, "channel");
  const summary = !capability.encoder
    ? "Encoder capability is still being measured."
    : capability.hardware
      ? `Hardware encoding · ${capability.encoder} · ${capacity}`
      : `Software encoding · ${capability.encoder} · ${capacity}`;

  return (
    <Stack
      align="center"
      className="border-border bg-static-900/40 border-b px-4 py-2.5"
      direction="row"
      gap="control"
      justify="between"
    >
      <div className="min-w-0">
        <p className="text-2xs text-muted-foreground">Encoder</p>
        <Text as="p" className="mt-0.5" textRole="compact" tone="secondary">
          {summary}
        </Text>
        {!!gpu.vramGiB && (
          <Text
            as="p"
            className="mt-0.5 flex flex-wrap items-baseline gap-x-2 gap-y-0.5"
            textRole="hint"
            tone="secondary"
          >
            <span>{gpu.name ?? "GPU"}</span>
            <span>{formatVram(gpu.vramGiB)} total</span>
            {gpu.llmModel && (
              <span>
                · {gpu.llmModel} holding {formatVram(gpu.llmVramGiB ?? 0)}
              </span>
            )}
          </Text>
        )}
      </div>
      {/* The whole reason the GPU header exists: a resident LLM sharing VRAM with the encoders is a
          real, silent cause of stutter, and nothing else on the dashboard says so. */}
      {gpu.contended && (
        <Badge shape="square" tone="caution">
          LLM sharing VRAM
        </Badge>
      )}
    </Stack>
  );
};

const ChannelRow = ({ channel }: { channel: ChannelHealth }) => {
  const slow = channel.speed > 0 && channel.speed < 1;
  return (
    <PanelRow
      meta={
        <>
          <div className="text-right">
            <Text as="p" textRole="mono" tone="primary">
              {formatColdStart(channel.coldStartMs)}
            </Text>
            <Text as="p" className="mt-0.5" textRole="mono">
              {formatBuffer(channel.bufferedMs)}
            </Text>
          </div>
          <Text
            as="span"
            className="w-14 text-right tabular-nums"
            textRole="mono"
            tone={slow ? "warning" : "secondary"}
          >
            {formatSpeed(channel.speed)}
          </Text>
          {/* A direct-play channel is REMUXED, not encoded — so "Hardware/Software" (an encoder
              distinction) is meaningless for it; it reads "Copy". Only a transcode has an encoder. */}
          {channel.mode === "direct-play" ? (
            <Badge shape="square" tone="neutral">
              Copy
            </Badge>
          ) : (
            <Badge shape="square" tone={channel.hardware ? "lock" : "caution"}>
              {channel.hardware ? "Hardware" : "Software"}
            </Badge>
          )}
          <Badge shape="square" tone={HEALTH_TONE[channel.health] ?? "neutral"}>
            {channel.health}
          </Badge>
        </>
      }
    >
      <Stack align="center" direction="row" gap="1.5">
        <Text as="p" className="truncate" textRole="cardLabel">
          {channelLabel(channel)}
        </Text>
        {/* The raw id lives behind an (i) affordance — noise on the row, useful for a support
              request. The design-system Tooltip gives a prompt, styled
              reveal on hover/focus, same idiom as FieldHelp; the aria-label keeps it screen-reader
              reachable. (A native `title` also "worked" but its ~1.5s browser delay read as broken.) */}
        <Tooltip content={<span className="font-mono text-xs">{channel.channelId}</span>}>
          <Action compact icon="info" layout="icon" variant="ghost">
            {`Channel id ${channel.channelId}`}
          </Action>
        </Tooltip>
      </Stack>
      <Text as="p" className="mt-0.5 truncate" textRole="hint" tone="secondary">
        {`${channel.target} · ${channel.mode}${channel.mode === "transcode" ? ` · ${channel.encoder}` : ""} · ${pluralize(channel.viewers, "viewer")}`}
      </Text>
    </PanelRow>
  );
};

const PlayoutPanel = ({ status, loading, title = "Playout" }: PlayoutPanelProps) => {
  const channels = status?.channels ?? [];

  return (
    <Card flexShrink={0} overflow="hidden">
      <Stack
        align="center"
        className="border-border border-b px-4 py-3"
        direction="row"
        gap="control"
        justify="between"
      >
        <Text as="h2" textRole="heading">
          {title}
        </Text>
        {!loading && status != null && (
          <Badge shape="square" tone="neutral">
            {status.running ? "Loomarr" : "Tunarr"}
          </Badge>
        )}
      </Stack>

      {loading && (
        <Text as="p" className="px-4 py-6" textRole="compact" tone="secondary">
          Reading playout status…
        </Text>
      )}

      {/* On a Tunarr-backed install there is no internal playout to show, which is not the same as
          every channel being unhealthy. */}
      {!loading && status != null && !status.running && (
        <Text as="p" className="px-4 py-6" textRole="compact" tone="secondary">
          Loomarr isn't streaming these channels. Tunarr is.
        </Text>
      )}

      {!loading && status?.running && <EncoderRow capability={status.capability} gpu={status.gpu} />}

      {!loading && status?.running && channels.length === 0 && (
        <Text as="p" className="px-4 py-6" textRole="compact" tone="secondary">
          No channels are streaming right now.
        </Text>
      )}

      {status?.running && channels.length > 0 && (
        <ListGroup framed={false}>
          {channels.map((c) => (
            <ChannelRow key={`${c.channelId}:${c.target}`} channel={c} />
          ))}
        </ListGroup>
      )}
    </Card>
  );
};

export { PlayoutPanel };
