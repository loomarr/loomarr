import {
  formatGuideEpisode,
  formatGuideTimeRange,
  type GuideAiringLayout,
  guideAiringLabel,
} from "@loomarr/core/guide";
import { type BadgeTone, Surface, Text } from "@loomarr/design-system";

import { ProgrammeCard } from "../../programme-card";
import type { GuideProgrammeDetailProps } from "./guide-detail.type";

const airingBadge = (kind: string, isOnNow: boolean): { label: string; tone: BadgeTone } => {
  if (kind === "pending") return { label: "Coming soon", tone: "warning" };
  if (kind === "filler") return { label: "Break", tone: "neutral" };
  if (kind === "flex") return { label: "Filler", tone: "neutral" };
  return isOnNow ? { label: "On now", tone: "live" } : { label: "Scheduled", tone: "neutral" };
};

const airingFacts = (airing: GuideAiringLayout) => {
  const source = airing.source;
  return [
    source.year ? String(source.year) : undefined,
    source.rating,
    source.genres?.slice(0, 2).join(" · "),
  ].filter((fact): fact is string => Boolean(fact));
};

// Authoritative programme facts shared by contextual and device-specific detail adapters.
const GuideProgrammeDetail = ({
  density = "pointer",
  focused = true,
  layout,
  minHeight = 180,
  renderArtwork,
  renderChannelLogo,
  selection,
}: GuideProgrammeDetailProps) => {
  const selectedChannel = selection
    ? layout.channels.find((channel) => channel.source.channelId === selection.channelId)
    : undefined;
  const selectedAiring = selectedChannel?.airings.find(
    (airing) => airing.scheduleBlockId === selection?.scheduleBlockId,
  );
  if (!selectedAiring || !selectedChannel) {
    return (
      <Surface alignItems="center" justifyContent="center" minHeight={minHeight} padding="$section">
        <Text density={density} textAlign="center" textRole="body">
          Choose a programme to see its details.
        </Text>
      </Surface>
    );
  }

  const artwork = renderArtwork?.(selectedAiring);
  const channelLogo = renderChannelLogo?.(selectedChannel);
  const source = selectedAiring.source;
  return (
    <Surface borderWidth={0} backgroundColor="$surfaceRaised" borderRadius="$cardCompact" overflow="hidden">
      <ProgrammeCard
        artwork={artwork}
        channelLogo={channelLogo}
        density={density}
        focused={focused}
        programme={{
          artworkState: (source.thumbImage || source.thumbUrl) && artwork ? "ready" : "missing",
          badge: airingBadge(source.kind, selectedAiring.isOnNow),
          channelLogoState: selectedChannel.source.logo && channelLogo ? "ready" : "missing",
          channelName: selectedChannel.source.name,
          channelNumber: String(selectedChannel.source.number),
          description: source.description,
          episodeLabel: formatGuideEpisode(source.season, source.episode),
          facts: airingFacts(selectedAiring),
          progressPercent:
            selectedAiring.progressRatio === undefined ? undefined : selectedAiring.progressRatio * 100,
          seriesTitle: source.series,
          timeLabel: formatGuideTimeRange(source.startMs, source.stopMs, layout.timezone),
          title: source.title.trim() || guideAiringLabel(source),
        }}
      />
      {source.kind === "filler" && source.pod?.entries.length ? (
        <Surface borderWidth={0} gap="$inline" padding="$control" accessibilityLabel="Break clips">
          {source.pod.entries.map((entry, index) => (
            // A clip can repeat within a break; its position distinguishes each airing.
            // biome-ignore lint/suspicious/noArrayIndexKey: position is identity within the pod
            <Text key={index} textRole="body">
              {entry.name} · {entry.kind.replaceAll("_", " ")}
            </Text>
          ))}
        </Surface>
      ) : null}
    </Surface>
  );
};

export { GuideProgrammeDetail };
