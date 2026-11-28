import { semanticColors } from "@loomarr/design-system";
import { Image, View } from "react-native";

// A programme's still or a spot's thumbnail on the playbar and its cards: always a picture, never
// flat colour (§9.2). Without one, the fallback tile is the artwork placeholder, so a missing still
// reads as "no picture", not as a colour that means something.

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
  left?: number;
  position?: "absolute";
  top?: number;
  width?: Length;
}

interface PlaybarStillProps {
  /** Fills its positioned parent, in place of `style`. */
  fill?: boolean;
  style?: PlaybarStillBox;
  uri?: string;
}

// An image takes no size from its insets alone on every client, so width and height are written out.
const FILL: PlaybarStillBox = { height: "100%", left: 0, position: "absolute", top: 0, width: "100%" };

const PlaybarStill = ({ fill = false, style, uri }: PlaybarStillProps) => {
  const box = fill ? FILL : style;
  return uri ? (
    <Image
      accessibilityIgnoresInvertColors
      resizeMode="cover"
      source={{ uri }}
      style={[{ backgroundColor: semanticColors.artwork.placeholder }, box]}
    />
  ) : (
    <View style={[{ backgroundColor: semanticColors.artwork.placeholder }, box]} />
  );
};

export type { PlaybarStillProps };
export { PlaybarStill };
