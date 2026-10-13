import { ProgressTrack, Surface, Text } from "@loomarr/design-system";
import { View } from "react-native";

import { ChannelIdent } from "../../channel-ident";
import type { SurfChannelCardProps } from "./surf-channel-card.type";

const identSize = 24;
// Number column plus the gaps on either side of it, so the secondary rows line up under the name.
const secondaryIndent = identSize + 12 + 20 + 12;

/**
 * The TV surf-list card: channel ident, number, name, live dot, and — when selected — the current
 * programme, time left and progress. One component so the surf rail's focused row and the channel
 * switch overlay are the same picture (#1458 reuses this card exactly rather than inventing one).
 */
const SurfChannelCard = ({ channel, current, logo, selected }: SurfChannelCardProps) => (
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
      {logo ? (
        <View style={{ height: identSize, overflow: "hidden", width: identSize }}>{logo}</View>
      ) : (
        <ChannelIdent
          name={channel.channelName}
          number={Number.parseInt(channel.channelNumber, 10) || 0}
          size={identSize}
        />
      )}
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
          paddingLeft={secondaryIndent}
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
        <Surface backgroundColor="$transparent" borderWidth={0} paddingLeft={secondaryIndent} paddingTop={4}>
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
