import type { ChannelHealth, DoctorGPU } from "@loomarr/api";
import { Badge, Card } from "@/components/ui";
import { cn } from "@/lib";
import type { PlayoutHealthPanelProps } from "./playout-health-panel.type";

// PlayoutHealthPanel — "is playout actually healthy right now" (§12 dashboard, GET
// /v1/playout/doctor).
//
// TranscodingPanel answers "how fast is each stream going"; this panel answers "why would a
// stream be slow" — and the single biggest cause on a homelab box is VRAM contention: an
// LLM sat resident on the same GPU the encoders need. §8.2 keeps a model warm between suggest
// calls for latency, which is invisible until playout also wants that card. `contended` is the
// backend's own correlation (a model resident AND channels transcoding on hardware) — this
// panel surfaces it, it doesn't recompute it.

// health/target/mode/encoder are free strings from the backend (§ no-hardcode) — never a
// closed enum on the frontend. Only the Badge TONE below is a closed mapping, and it degrades
// safely: an unrecognised health string still renders, just untinted.
const HEALTH_TONE: Record<string, "lock" | "caution"> = {
  ok: "lock",
  degraded: "caution",
  stalled: "caution",
};

// formatSpeed mirrors TranscodingPanel: "—" for a channel with no sample yet (direct-play
// copies report 0 rather than a rate), never "0.0× rt", which reads as stalled.
const formatSpeed = (speed: number): string => (speed > 0 ? `${speed.toFixed(1)}× rt` : "—");

const formatVram = (gib: number): string => `${gib.toFixed(1)} GB`;

const GpuRow = ({ gpu }: { gpu: DoctorGPU }) => {
  if (!gpu.vramGiB) {
    return (
      <div className="border-border border-b px-4 py-2.5 text-muted-foreground text-sm">
        Software encoding (no GPU)
      </div>
    );
  }

  return (
    <div className="flex items-center justify-between gap-3 border-border border-b px-4 py-2.5">
      <div className="min-w-0">
        <p className="truncate text-sm">{gpu.name ?? "GPU"}</p>
        <p className="mt-0.5 text-muted-foreground text-xs">
          {gpu.llmModel
            ? `${gpu.llmModel} · ${formatVram(gpu.llmVramGiB ?? 0)} resident · ${formatVram(gpu.vramGiB)} total`
            : `${formatVram(gpu.vramGiB)} total`}
        </p>
      </div>
      {/* The whole reason this panel exists: a resident LLM sharing VRAM with the encoders
          is a real, silent cause of stutter, and nothing else on the dashboard says so. */}
      {gpu.contended && <Badge variant="caution">LLM sharing VRAM</Badge>}
    </div>
  );
};

const ChannelRow = ({ channel }: { channel: ChannelHealth }) => (
  <li className="flex items-center gap-3 border-border border-b px-4 py-2.5 last:border-b-0">
    <div className="min-w-0 flex-1">
      <p className="truncate font-mono text-sm">{channel.channelId}</p>
      <p className="mt-0.5 truncate text-muted-foreground text-xs">
        {`${channel.target} · ${channel.mode} · ${channel.reason}`}
      </p>
    </div>
    <span
      className={cn(
        "w-16 shrink-0 text-right font-mono text-xs",
        channel.speed > 0 && channel.speed < 1 ? "text-caution" : "text-muted-foreground",
      )}
    >
      {formatSpeed(channel.speed)}
    </span>
    <Badge variant={HEALTH_TONE[channel.health] ?? "neutral"}>{channel.health}</Badge>
  </li>
);

const PlayoutHealthPanel = ({ doctor, loading, className }: PlayoutHealthPanelProps) => {
  const channels = doctor?.channels ?? [];

  return (
    <Card className={cn("flex flex-col overflow-hidden p-0", className)}>
      <div className="flex items-center justify-between gap-3 border-border border-b px-4 py-3">
        <h2 className="font-semibold text-base">Playout health</h2>
      </div>

      {loading && <p className="px-4 py-6 text-muted-foreground text-sm">Reading playout health…</p>}

      {/* ⚠ Same distinction TranscodingPanel draws: on a Tunarr-backed install there is no
          internal playout to diagnose, which is not the same as every channel being unhealthy. */}
      {!loading && doctor != null && !doctor.running && (
        <p className="px-4 py-6 text-muted-foreground text-sm">
          Loomarr isn't streaming these channels. Tunarr is.
        </p>
      )}

      {!loading && doctor?.running && <GpuRow gpu={doctor.gpu} />}

      {!loading && doctor?.running && channels.length === 0 && (
        <p className="px-4 py-6 text-muted-foreground text-sm">Nothing is being watched right now.</p>
      )}

      {doctor?.running && channels.length > 0 && (
        <ul className="flex flex-col">
          {channels.map((c) => (
            <ChannelRow key={c.channelId} channel={c} />
          ))}
        </ul>
      )}
    </Card>
  );
};

export { PlayoutHealthPanel };
