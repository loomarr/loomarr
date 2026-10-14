import { type ChannelIdentHue, channelIdentHue, monogramOf } from "@loomarr/core/guide";
import { brandChroma, Text } from "@loomarr/design-system";
import { Platform, View } from "react-native";
import type { ChannelIdentProps } from "./channel-ident.type";

const hueHex: Record<ChannelIdentHue, string> = {
  tune: brandChroma[3],
  suggest: brandChroma[4],
  signal: brandChroma[0],
  onair: brandChroma[5],
};

const withAlpha = (hex: string, alpha: number) => {
  const n = Number.parseInt(hex.slice(1), 16);
  return `rgba(${(n >> 16) & 255}, ${(n >> 8) & 255}, ${n & 255}, ${alpha})`;
};

// The hatch the mock lays behind a monogram. CSS on web; native keeps the flat tint.
const hatch = "repeating-linear-gradient(135deg, transparent 0 3px, rgba(255, 255, 255, 0.05) 3px 6px)";

// The size the guide draws, which the monogram's type role is set for.
const BASE_SIZE = 30;

// ChannelIdent is a channel's identity when it has no icon (N8): its initials in a tile tinted by
// the channel number's hue. Decorative: the channel's name is always said beside it.
const ChannelIdent = ({ name, number, size = BASE_SIZE }: ChannelIdentProps) => {
  const hex = hueHex[channelIdentHue(number)];
  const scale = size / BASE_SIZE;
  return (
    <View
      aria-hidden
      style={[
        {
          alignItems: "center",
          backgroundColor: withAlpha(hex, 0.08),
          borderColor: withAlpha(hex, 0.3),
          borderRadius: Math.round(5 * scale),
          borderWidth: 1,
          height: size,
          justifyContent: "center",
          width: size,
        },
        Platform.OS === "web" ? ({ backgroundImage: hatch } as object) : null,
      ]}
    >
      {/* The hue is per channel, so it can't be a text tone. */}
      <Text
        style={[{ color: hex }, scale === 1 ? null : { fontSize: 11 * scale, lineHeight: 14 * scale }]}
        textRole="guideIdent"
      >
        {monogramOf(name)}
      </Text>
    </View>
  );
};

export { ChannelIdent };
