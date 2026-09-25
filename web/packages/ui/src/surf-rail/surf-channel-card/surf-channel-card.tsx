import { ProgressTrack, Surface, Text } from "@loomarr/design-system";
import type { SurfChannelCardProps } from "./surf-channel-card.type";

/**
 * The TV surf-list card: channel number, name, live dot, and — when selected — the current
 * programme, time left and progress. One component so the surf rail's focused row and the channel
 * switch overlay are the same picture (#1458 reuses this card exactly rather than inventing one).
 */
const SurfChannelCard = ({ channel, current, selected }: SurfChannelCardProps) => (
  <Surface
    backgroundColor={selected ? "$surfaceRaised" : "$surfaceCanvas"}
    borderColor={selected ? "$actionFocus" : "$surfaceCanvas"}
    borderRadius={12}
    borderWidth={selected ? 3 : 1}
    gap={4}
    paddingHorizontal={16}
    paddingVertical={12}
  >
    <Surface alignItems="center" backgroundColor="$transparent" borderWidth={0} flexDirection="row" gap={12}>
      <Text density="tv" textRole="data" tone={selected ? "signal" : "muted"}>
        {channel.channelNumber.padStart(2, "0")}
      </Text>
      <Text density="tv" flex={1} numberOfLines={1} textRole="compact">
        {channel.channelName}
      </Text>
      {current ? (
        <Surface backgroundColor="$stateSuccess" borderRadius="$round" borderWidth={0} height={8} width={8} />
      ) : null}
    </Surface>
    {selected ? (
      <>
        <Surface
          alignItems="center"
          backgroundColor="$transparent"
          borderWidth={0}
          flexDirection="row"
          gap="$inline"
          paddingLeft={32}
        >
          <Text density="tv" flex={1} numberOfLines={1} textRole="caption" tone="muted">
            {channel.now?.seriesTitle
              ? channel.now.title.replace(`${channel.now.seriesTitle} · `, "")
              : (channel.now?.title ?? "Nothing scheduled")}
          </Text>
          {channel.now?.remainingLabel ? (
            <Text density="tv" textRole="metadata" tone="muted">
              {channel.now.remainingLabel}
            </Text>
          ) : null}
        </Surface>
        <Surface backgroundColor="$transparent" borderWidth={0} paddingLeft={32} paddingTop={4}>
          <ProgressTrack
            accessibilityLabel={channel.now?.title ?? channel.channelName}
            percent={channel.now?.progressPercent ?? 0}
            width="100%"
          />
        </Surface>
      </>
    ) : null}
  </Surface>
);

export { SurfChannelCard };
