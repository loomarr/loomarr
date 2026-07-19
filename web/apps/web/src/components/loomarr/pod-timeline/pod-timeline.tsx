import type { PodEntryDTO, PreviewPodsOutputBodyMatchLevel } from "@loomarr/api";
import { formatClipDuration } from "@loomarr/core";
import { Badge } from "@/components/ui";
import { cn } from "@/lib";
import type { PodTimelineProps } from "./pod-timeline.type";

// PodTimeline — a commercial break made legible (§3, §10): bumper → ads → bumper, each
// segment sized by its duration, with the pod's era/audience context.
//
// The match chip answers "why are my commercials wrong". `exact` is the quiet case and
// shows nothing; every other level says how far down the §10 fallback ladder assembly had
// to go, ending at `bumper_card` — the dead-air guard, where no ad matched and the
// embedded card stands in rather than silence.
const MATCH: Record<
  PreviewPodsOutputBodyMatchLevel,
  { label: string; variant: "caution" | "neutral" } | null
> = {
  exact: null,
  widened: { label: "Era widened", variant: "caution" },
  audience: { label: "Era ignored", variant: "caution" },
  bumper_card: { label: "Bumper only", variant: "neutral" },
};

const SEGMENT_FILL: Record<PodEntryDTO["kind"], string> = {
  bumper: "bg-tune-tint-30",
  station_id: "bg-tune-tint-30",
  commercial: "bg-suggest-tint-30",
  trailer: "bg-signal-tint-30",
  psa: "bg-signal-tint-30",
  interstitial: "bg-static-700",
};

const SEGMENT_ABBR: Record<PodEntryDTO["kind"], string> = {
  bumper: "BMP",
  station_id: "ID",
  commercial: "AD",
  trailer: "TRL",
  psa: "PSA",
  interstitial: "INT",
};

const PodTimeline = ({ entries, matchLevel = "exact", era, audience, className }: PodTimelineProps) => {
  const total = entries.reduce((sum, e) => sum + e.durationMs, 0) || 1;
  const chip = MATCH[matchLevel];
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
        {entries.map((entry, i) => (
          <li
            // A pod is an ordered, immutable sequence — position IS identity. The same
            // clip can legitimately appear twice in one pod, and the embedded fallback
            // card has no program id at all, so nothing else uniquely identifies a
            // segment. The list is replaced whole on every preview rather than mutated,
            // which is the reordering case the rule guards against.
            // biome-ignore lint/suspicious/noArrayIndexKey: position is identity in a pod
            key={`${entry.tunarrProgramId ?? "card"}-${i}`}
            title={`${entry.name} · ${formatClipDuration(entry.durationMs)}`}
            style={{ flexBasis: `${(entry.durationMs / total) * 100}%` }}
            className={cn(
              "flex min-w-0 items-center justify-center border-border border-r px-1 last:border-r-0",
              SEGMENT_FILL[entry.kind],
            )}
          >
            <span className="truncate font-mono text-[10px] text-static-100 uppercase tracking-wide">
              {SEGMENT_ABBR[entry.kind]}
            </span>
          </li>
        ))}
      </ul>
    </div>
  );
};

export { PodTimeline };
