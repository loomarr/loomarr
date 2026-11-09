import type { PodEntryDTO } from "@loomarr/api/models/podEntryDTO";
import type { PodPoolDTOMatchLevel } from "@loomarr/api/models/podPoolDTOMatchLevel";
import { formatClipDuration } from "@loomarr/core/format";
import { guideFillerEntryLabel } from "@loomarr/core/guide";
import { Badge, type BadgeTone, Text } from "@loomarr/design-system";
import { cn } from "@/lib/utils";
import type { PodTimelineProps } from "./pod-timeline.type";

// PodTimeline — a commercial break made legible (§3, §10): bumper → ads → bumper, each
// segment sized by its duration, with the pod's era/audience context.
//
// The match chip answers "why are my commercials wrong". `exact` is the quiet case and
// shows nothing; every other level says how far down the §10 fallback ladder assembly had
// to go, ending at `bumper_card` — the dead-air guard, where no ad matched and the
// embedded card stands in rather than silence.
const MATCH: Record<PodPoolDTOMatchLevel, { label: string; tone: BadgeTone } | null> = {
  exact: null,
  widened: { label: "Era widened", tone: "caution" },
  audience: { label: "Any era", tone: "caution" },
  bumper_card: { label: "Bumper only", tone: "neutral" },
};

// A member-readable pod withholds a name that is only a filename (#1980); the segment then reads
// as its role ("Commercial").
const clipName = (entry: PodEntryDTO): string => entry.name.trim() || guideFillerEntryLabel(entry);

const SEGMENT_FILL: Record<PodEntryDTO["role"], string> = {
  bumper: "bg-tune-tint-30",
  station_id: "bg-tune-tint-30",
  commercial: "bg-suggest-tint-30",
  trailer: "bg-signal-tint-30",
  psa: "bg-signal-tint-30",
  interstitial: "bg-static-700",
  // A political ad is an ad, and news is an interstitial (#2037): each reads as its family.
  political: "bg-suggest-tint-30",
  news: "bg-static-700",
};

// Each segment prints its role in words and its length ("Trailer, 41s"), so no legend is needed.
// A segment is sized by its duration and may be narrow: the label truncates and the full text is
// still on the segment's title and in its screen-reader text.
const SEGMENT_WORD: Record<PodEntryDTO["role"], string> = {
  bumper: "Bumper",
  station_id: "Station ID",
  commercial: "Commercial",
  trailer: "Trailer",
  psa: "Public service ad",
  interstitial: "Interstitial",
  political: "Political ad",
  news: "News",
};

const segmentLabel = (entry: PodEntryDTO): string =>
  `${SEGMENT_WORD[entry.role]}, ${formatClipDuration(entry.durationMs)}`;

const PodTimeline = ({
  entries,
  matchLevel = "exact",
  era,
  audience,
  className,
  onPreview,
}: PodTimelineProps) => {
  const total = entries.reduce((sum, e) => sum + e.durationMs, 0) || 1;
  const chip = MATCH[matchLevel];
  return (
    <div className={cn("flex flex-col gap-2", className)}>
      <div className="flex flex-wrap items-center gap-2">
        <span className="text-static-400 text-xs">{`Pod · ${formatClipDuration(total)}`}</span>
        {era ? <Badge shape="square" tone="neutral">{`${era}s`}</Badge> : null}
        {audience ? (
          <Badge shape="square" tone="neutral">
            {audience}
          </Badge>
        ) : null}
        {chip && (
          <Badge shape="square" tone={chip.tone}>
            {chip.label}
          </Badge>
        )}
      </div>

      <ul
        className="border-border flex h-9 w-full overflow-hidden rounded-md border"
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
            key={`${entry.hash ?? entry.path ?? "card"}-${i}`}
            title={`${clipName(entry)} · ${formatClipDuration(entry.durationMs)}`}
            style={{ flexBasis: `${(entry.durationMs / total) * 100}%` }}
            className={cn("border-border min-w-0 border-r last:border-r-0", SEGMENT_FILL[entry.role])}
          >
            {onPreview && entry.hash && !entry.isFallbackCard ? (
              <button
                type="button"
                aria-label={`Preview ${clipName(entry)}`}
                className="focus-visible:ring-ring flex h-full w-full min-w-0 cursor-pointer items-center justify-center px-1 outline-none hover:brightness-110 focus-visible:ring-2 focus-visible:ring-inset"
                onClick={() => onPreview(entry)}
              >
                <span className="sr-only">{`${clipName(entry)}, ${formatClipDuration(entry.durationMs)}`}</span>
                <Text aria-hidden as="span" className="truncate" textRole="caption" tone="primary">
                  {segmentLabel(entry)}
                </Text>
              </button>
            ) : (
              <span className="flex h-full min-w-0 items-center justify-center px-1">
                <span className="sr-only">{`${clipName(entry)}, ${formatClipDuration(entry.durationMs)}`}</span>
                <Text aria-hidden as="span" className="truncate" textRole="caption" tone="primary">
                  {segmentLabel(entry)}
                </Text>
              </span>
            )}
          </li>
        ))}
      </ul>
    </div>
  );
};

export { PodTimeline };
