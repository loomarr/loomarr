import { channelIdentHue, monogramOf } from "@loomarr/core/guide";
import { CHANNEL_IDENT_WASH_PERCENT, channelIdentHues, Text } from "@loomarr/design-system";
import { Image, View } from "react-native";
import type { ChannelIdentProps } from "./channel-ident.type";

const withAlpha = (hex: string, alpha: number) => {
  const n = Number.parseInt(hex.slice(1), 16);
  return `rgba(${(n >> 16) & 255}, ${(n >> 8) & 255}, ${n & 255}, ${alpha})`;
};

// The size the guide draws, which the monogram's type role is set for.
const BASE_SIZE = 30;

// ChannelIdent is a channel's identity when it has no icon (N8): its initials in a tile washed in
// the channel number's hue. The wash is the brand hue at 14%, flat (no hatch), and the letters take
// that hue's on-tint text stop for the theme, so every hue clears AA in both themes (#3011).
// A channel with an icon draws it in the same tile instead, labelled with the channel's name.
// Otherwise decorative: the channel's name is always said beside it.
const ChannelIdent = ({ name, number, logo, size = BASE_SIZE }: ChannelIdentProps) => {
  const { wash, tone } = channelIdentHues[channelIdentHue(number)];
  const scale = size / BASE_SIZE;
  if (logo) {
    return (
      <View
        style={{
          borderColor: withAlpha(wash, 0.3),
          borderRadius: Math.round(5 * scale),
          borderWidth: 1,
          height: size,
          overflow: "hidden",
          width: size,
        }}
      >
        <Image
          accessibilityLabel={`${name} logo`}
          resizeMode="cover"
          role="img"
          source={{ uri: logo }}
          style={{ height: "100%", width: "100%" }}
        />
      </View>
    );
  }
  return (
    <View
      aria-hidden
      style={{
        alignItems: "center",
        backgroundColor: withAlpha(wash, CHANNEL_IDENT_WASH_PERCENT / 100),
        borderColor: withAlpha(wash, 0.3),
        borderRadius: Math.round(5 * scale),
        borderWidth: 1,
        height: size,
        justifyContent: "center",
        width: size,
      }}
    >
      <Text
        strong
        style={scale === 1 ? undefined : { fontSize: 12 * scale, lineHeight: 16 * scale }}
        textRole="label"
        tone={tone}
      >
        {monogramOf(name)}
      </Text>
    </View>
  );
};

export { ChannelIdent };
