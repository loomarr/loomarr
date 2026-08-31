import { Badge, Surface, semanticTargets, Text } from "@loomarr/design-system";

import type { ChannelIdentityProps, ProgrammeIdentityProps } from "./identity.type";

const channelInitials = (name: string) =>
  name
    .split(/\s+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((word) => word[0]?.toUpperCase())
    .join("");

const ChannelIdentity = ({ channel, density = "pointer", logo }: ChannelIdentityProps) => {
  const logoReady = channel.channelLogoState === "ready" && logo;
  const logoSize = semanticTargets[density];
  return (
    <Surface
      alignItems="center"
      backgroundColor="$transparent"
      borderWidth={0}
      flexDirection="row"
      gap="$control"
      minWidth={0}
    >
      <Surface
        alignItems="center"
        backgroundColor="$surfaceElevated"
        borderRadius="$control"
        height={logoSize}
        justifyContent="center"
        overflow="hidden"
        width={logoSize}
      >
        {logoReady ? (
          logo
        ) : (
          <Text density={density} textRole="label" tone="secondary">
            {channelInitials(channel.channelName)}
          </Text>
        )}
      </Surface>
      <Text density={density} textRole="channelNumber">
        {channel.channelNumber}
      </Text>
      <Surface backgroundColor="$transparent" borderWidth={0} flex={1} gap={2} minWidth={0}>
        <Text density={density} numberOfLines={1} textRole="label" tone="primary">
          {channel.channelName}
        </Text>
      </Surface>
    </Surface>
  );
};

const ProgrammeIdentity = ({ density = "pointer", programme }: ProgrammeIdentityProps) => (
  <Surface backgroundColor="$transparent" borderWidth={0} gap="$control" minWidth={0}>
    <Surface
      backgroundColor="$transparent"
      borderWidth={0}
      flexDirection="row"
      gap="$control"
      justifyContent="space-between"
      minWidth={0}
    >
      <Surface backgroundColor="$transparent" borderWidth={0} flex={1} gap={4} minWidth={0}>
        {programme.seriesTitle ? (
          <Text density={density} numberOfLines={1} textRole="label">
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

    <Text density={density} textRole="time">
      {[programme.timeLabel, programme.episodeLabel].filter(Boolean).join(" · ")}
    </Text>

    {programme.facts?.length ? (
      <Text density={density} textRole="metadata">
        {programme.facts.join(" · ")}
      </Text>
    ) : null}

    {programme.description ? (
      <Text density={density} numberOfLines={density === "tv" ? 3 : 2} textRole="body">
        {programme.description}
      </Text>
    ) : null}
  </Surface>
);

export { ChannelIdentity, ProgrammeIdentity };
