import { ArtworkFrame, Badge, FocusSurface, ProgressTrack, Surface, Text } from "@loomarr/design-system";

import type { ProgrammeCardProps } from "./programme-card.type";

const ProgrammeCard = ({ artwork, density = "pointer", focused = false, programme }: ProgrammeCardProps) => {
  const padding = density === "tv" ? 24 : 16;
  const maxWidth = density === "tv" ? 760 : density === "touch" ? 620 : 560;

  return (
    <FocusSurface focused={focused} gap="$control" maxWidth={maxWidth} padding={padding} width="100%">
      <ArtworkFrame density={density} state={programme.artworkState} width="100%">
        {artwork}
      </ArtworkFrame>

      <Surface
        backgroundColor="$transparent"
        borderWidth={0}
        flexDirection="row"
        gap="$control"
        justifyContent="space-between"
      >
        <Surface backgroundColor="$transparent" borderWidth={0} flex={1} gap={4}>
          {programme.seriesTitle ? (
            <Text density={density} textRole="label">
              {programme.seriesTitle}
            </Text>
          ) : null}
          <Text density={density} numberOfLines={2} textRole="title">
            {programme.title}
          </Text>
        </Surface>
        {programme.badge ? (
          <Badge density={density} tone={programme.badge.tone}>
            {programme.badge.label}
          </Badge>
        ) : null}
      </Surface>

      <Surface
        alignItems="center"
        backgroundColor="$transparent"
        borderWidth={0}
        flexDirection="row"
        gap="$inline"
      >
        <Text density={density} textRole="channelNumber">
          {programme.channelNumber}
        </Text>
        <Surface backgroundColor="$transparent" borderWidth={0} flex={1} gap={2}>
          <Text density={density} textRole="label">
            {programme.channelName}
          </Text>
          <Text density={density} textRole="time">
            {[programme.timeLabel, programme.episodeLabel].filter(Boolean).join(" · ")}
          </Text>
        </Surface>
      </Surface>

      {programme.description ? (
        <Text density={density} numberOfLines={density === "tv" ? 3 : 2} textRole="body">
          {programme.description}
        </Text>
      ) : null}

      {programme.progressPercent === undefined ? null : (
        <ProgressTrack percent={programme.progressPercent} width="100%" />
      )}
    </FocusSurface>
  );
};

export { ProgrammeCard };
