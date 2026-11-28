import { formatGuideEpisode, type GuideAiringLayout } from "@loomarr/core/guide";
import { Surface, Text, ThemeScope } from "@loomarr/design-system";
import { View } from "react-native";

interface ProgrammeFactsProps {
  airing: GuideAiringLayout["source"];
  formatTime: (ms: number) => string;
  /** `picture` stands the facts on a solid ink plate; `page` sets them on the page, as under the Guide. */
  ground?: "page" | "picture";
  nowMs: number;
  /** The block's own span, where it differs from the airing's (a run of one series). */
  startMs?: number;
  stopMs?: number;
}

/**
 * A programme's facts as type (frontend-design §9.2, The Cross): title, episode, rating, genres, a
 * synopsis of a few lines, and when it starts or until when it runs.
 */
const ProgrammeFacts = ({
  airing,
  formatTime,
  ground = "page",
  nowMs,
  startMs = airing.startMs,
  stopMs = airing.stopMs,
}: ProgrammeFactsProps) => {
  const series = airing.series?.trim();
  const episodeName = series ? airing.title.trim() : "";
  const title = series || airing.title.trim();
  const episode = [formatGuideEpisode(airing.season, airing.episode), episodeName]
    .filter(Boolean)
    .join(" · ");
  const when =
    stopMs <= nowMs
      ? `Ended ${formatTime(stopMs)}`
      : startMs > nowMs
        ? `Starts ${formatTime(startMs)}`
        : `Until ${formatTime(stopMs)}`;
  const facts = [airing.rating, airing.genres?.join(", "), when].filter(Boolean).join(" · ");

  const body = (
    <View aria-label="Programme facts" role="group" style={{ gap: 4, maxWidth: 520, minWidth: 0 }}>
      <Text numberOfLines={2} textRole="title">
        {title}
      </Text>
      {episode ? (
        <Text numberOfLines={1} textRole="label">
          {episode}
        </Text>
      ) : null}
      <Text numberOfLines={2} textRole="mono">
        {facts}
      </Text>
      {airing.description ? (
        <Text numberOfLines={3} textRole="body">
          {airing.description}
        </Text>
      ) : null}
    </View>
  );

  if (ground === "page") return body;
  return (
    <ThemeScope theme="dark">
      <Surface
        alignSelf="flex-start"
        backgroundColor="surfaceCanvas"
        borderRadius={8}
        borderWidth={0}
        maxWidth="100%"
        padding={16}
        testID="programme-facts-plate"
      >
        {body}
      </Surface>
    </ThemeScope>
  );
};

export type { ProgrammeFactsProps };
export { ProgrammeFacts };
