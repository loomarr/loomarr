import { semanticColors } from "@loomarr/design-system";
import { Image, View } from "react-native";

import { webOnly } from "../glass";

// A programme's still or a spot's thumbnail on the playbar and its cards: always a picture, never
// flat colour (§9.2). Without one, the fallback tile is the artwork placeholder under the channel
// ident's hatch, so a missing still reads as "no picture", not as a colour that means something.

const hatch = "repeating-linear-gradient(135deg, transparent 0 3px, rgba(255, 255, 255, 0.05) 3px 6px)";

type Length = number | `${number}%`;

// ⚠ The box the still fills in its caller's layout, named field by field rather than as React
// Native's ViewStyle: Storybook's docgen follows a component's prop types into their imports, and
// it cannot parse react-native's own source (the build fails on react-native/index.js).
interface PlaybarStillBox {
  aspectRatio?: number;
  borderRadius?: number;
  flexBasis?: Length;
  flexGrow?: number;
  height?: Length;
  position?: "absolute";
  width?: Length;
}

interface PlaybarStillProps {
  style?: PlaybarStillBox;
  uri?: string;
}

const PlaybarStill = ({ style, uri }: PlaybarStillProps) =>
  uri ? (
    <Image
      accessibilityIgnoresInvertColors
      resizeMode="cover"
      source={{ uri }}
      style={[{ backgroundColor: semanticColors.artwork.placeholder }, style]}
    />
  ) : (
    <View
      style={[
        { backgroundColor: semanticColors.artwork.placeholder, ...webOnly({ backgroundImage: hatch }) },
        style,
      ]}
    />
  );

export type { PlaybarStillProps };
export { PlaybarStill };
