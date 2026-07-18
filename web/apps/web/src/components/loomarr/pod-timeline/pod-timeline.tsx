import type { ClipDTO } from "@loomarr/api";
import { formatClipDuration } from "@loomarr/core";
import { Badge } from "@/components/ui";
import { cn } from "@/lib";
import type { PodMatch, PodTimelineProps } from "./pod-timeline.type";

// PodTimeline — a commercial break made legible (§3, §10): bumper → ads → bumper,
// each segment sized by its duration, with the pod's era/audience context. `matched`
// means every clip hit the pod's target; `fallback-widened` flags that the relaxation
// ladder loosened to fill the break (caution); `bumper-card-only` is the dead-air
// guard — a bumper card stands in when no ads matched, never silence (§10).
const MATCH: Record<PodMatch, { label: string; variant: "caution" | "neutral" } | null> = {
  matched: null,
  "fallback-widened": { label: "Widened match", variant: "caution" },
  "bumper-card-only": { label: "Bumper only", variant: "neutral" },
};

const SEGMENT_FILL: Record<ClipDTO["kind"], string> = {
  bumper: "bg-tune-tint-30",
  station_id: "bg-tune-tint-30",
  commercial: "bg-suggest-tint-30",
  trailer: "bg-signal-tint-30",
  psa: "bg-signal-tint-30",
  interstitial: "bg-static-700",
};

const SEGMENT_ABBR: Record<ClipDTO["kind"], string> = {
  bumper: "BMP",
  station_id: "ID",
  commercial: "AD",
  trailer: "TRL",
  psa: "PSA",
  interstitial: "INT",
};

const PodTimeline = ({ clips, match = "matched", era, audience, className }: PodTimelineProps) => {
  const total = clips.reduce((sum, c) => sum + c.durationMs, 0) || 1;
  const chip = MATCH[match];
  const segments = clips.map((clip, i) => ({ key: `${clip.name}-${clip.kind}-${i}`, clip }));
  return (
    <div className={cn("flex flex-col gap-2", className)}>
      <div className="flex flex-wrap items-center gap-2">
        <span className="font-mono text-static-400 text-xs uppercase tracking-wide">
          {`Pod · ${formatClipDuration(total)}`}
        </span>
        {era ? <Badge variant="neutral">{`${era}s`}</Badge> : null}
        {audience ? <Badge variant="neutral">{audience}</Badge> : null}
        {chip && <Badge variant={chip.variant}>{chip.label}</Badge>}
      </div>

      <ul
        className="flex h-9 w-full overflow-hidden rounded-md border border-border"
        aria-label="Pod segments"
      >
        {segments.map(({ key, clip }) => (
          <li
            key={key}
            title={`${clip.name} · ${formatClipDuration(clip.durationMs)}`}
            style={{ flexBasis: `${(clip.durationMs / total) * 100}%` }}
            className={cn(
              "flex min-w-0 items-center justify-center border-border border-r px-1 last:border-r-0",
              SEGMENT_FILL[clip.kind],
            )}
          >
            <span className="truncate font-mono text-[10px] text-static-100 uppercase tracking-wide">
              {SEGMENT_ABBR[clip.kind]}
            </span>
          </li>
        ))}
      </ul>
    </div>
  );
};

export { PodTimeline };
