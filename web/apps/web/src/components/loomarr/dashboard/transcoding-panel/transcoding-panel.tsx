import type { SessionStat } from "@loomarr/api";
import { pluralize } from "@loomarr/core";
import { Badge, Card } from "@/components/ui";
import { cn } from "@/lib";
import type { TranscodingPanelProps } from "./transcoding-panel.type";

// TranscodingPanel — live internal-playout telemetry (§12 dashboard, V16).
//
// Every number here is MEASURED, not estimated. `playout.Start` has parsed ffmpeg's structured
// progress (`-progress pipe:3`) since the process supervisor was written, and every caller
// passed nil for the callback — so speed and timing were parsed and thrown away on every
// stream. V16 wires that through; this panel is where it surfaces.
//
// Fed by GET /v1/playout/sessions and refreshed by the `playout` SSE frame, which fires when a
// channel starts or stops. Not per progress sample: those arrive ~1/s per stream and would push
// several frames a second at every open browser for numbers that move by fractions (§8).

// A sustained speed below 1.0 means the encoder is losing to realtime and the channel will
// stutter — the single most useful health signal ffmpeg gives us. Slightly above 1.0 is normal
// on a busy box; the warning threshold sits a little under to avoid crying wolf at 1.02.
const SPEED_WARN = 1.05;

// formatSpeed renders the realtime multiple. "—" while a fresh encoder has not reported yet,
// rather than "0× rt", which reads as "stalled" when it means "no sample".
const formatSpeed = (speed: number): string => (speed > 0 ? `${speed.toFixed(1)}× rt` : "—");

// formatBuffer says how far ahead of realtime the encoder has produced output. Negative means
// it is BEHIND, which is the same condition a sub-1.0 speed reports, seen as accumulated
// deficit rather than an instantaneous rate.
const formatBuffer = (ms: number): string => {
  const secs = Math.round(ms / 1000);
  if (secs < 0) return `${Math.abs(secs)}s behind`;
  return `encoded ${secs}s ahead`;
};

const StreamRow = ({ stat }: { stat: SessionStat }) => {
  const slow = stat.speed > 0 && stat.speed < SPEED_WARN;
  return (
    <li className="flex items-center gap-3 border-border border-b px-4 py-2.5 last:border-b-0">
      <div className="min-w-0 flex-1">
        <p className="truncate font-mono text-sm">{stat.channelId}</p>
        <p className="mt-0.5 text-muted-foreground text-xs">
          {`${pluralize(stat.viewers, "viewer")} · ${formatBuffer(stat.bufferedMs)}`}
        </p>
      </div>
      {/* The most actionable field on the panel: hardware vs software is the difference
          between four concurrent streams and one. */}
      <Badge variant={stat.hardware ? "lock" : "caution"}>{stat.hardware ? "Hardware" : "Software"}</Badge>
      <span
        className={cn(
          "w-20 shrink-0 text-right font-mono text-xs",
          slow ? "text-caution" : "text-muted-foreground",
        )}
      >
        {formatSpeed(stat.speed)}
      </span>
    </li>
  );
};

const TranscodingPanel = ({ telemetry, loading, className }: TranscodingPanelProps) => {
  const sessions = telemetry?.sessions ?? [];
  const atCapacity = telemetry != null && telemetry.capacity > 0 && telemetry.active >= telemetry.capacity;

  return (
    <Card className={cn("flex flex-col overflow-hidden p-0", className)}>
      <div className="flex items-center justify-between gap-3 border-border border-b px-4 py-3">
        <h2 className="font-semibold text-base">Transcoding</h2>
        {telemetry?.running && (
          <span className={cn("font-mono text-sm", atCapacity ? "text-caution" : "text-muted-foreground")}>
            {`${telemetry.active} / ${telemetry.capacity}`}
          </span>
        )}
      </div>

      {loading && <p className="px-4 py-6 text-muted-foreground text-sm">Reading encoder state…</p>}

      {/* ⚠ "Not running" is NOT "nothing playing". On a Tunarr-backed install the session list
          is legitimately empty, and a panel that cannot tell those apart renders a blank table
          that reads as every channel having just died. The backend sends the flag explicitly so
          this distinction is possible at all. */}
      {!loading && telemetry != null && !telemetry.running && (
        <p className="px-4 py-6 text-muted-foreground text-sm">
          Loomarr isn't streaming these channels — Tunarr is. Per-stream telemetry appears here for channels
          Loomarr plays itself.
        </p>
      )}

      {!loading && telemetry?.running && sessions.length === 0 && (
        <p className="px-4 py-6 text-muted-foreground text-sm">
          Nothing is being watched right now. A channel starts encoding when someone tunes in.
        </p>
      )}

      {sessions.length > 0 && (
        <ul className="flex flex-col">
          {sessions.map((s) => (
            <StreamRow key={s.channelId} stat={s} />
          ))}
        </ul>
      )}
    </Card>
  );
};

export { TranscodingPanel };
