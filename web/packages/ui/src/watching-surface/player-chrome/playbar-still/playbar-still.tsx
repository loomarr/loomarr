import { semanticColors } from "@loomarr/design-system";
import { createContext, useContext } from "react";
import { Image, type ImageURISource, View } from "react-native";

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

/**
 * How a still's path becomes an image source. A browser loads the server's path as it is; a paired
 * native client resolves it against its server and signs it, and returns undefined for a path it
 * must not send its token to, which draws the fallback tile.
 */
type PlaybarStillSource = (uri: string) => ImageURISource | undefined;

const PlaybarStillSourceContext = createContext<PlaybarStillSource>((uri) => ({ uri }));

interface PlaybarStillProps {
  style?: PlaybarStillBox;
  uri?: string;
}

const PlaybarStill = ({ style, uri }: PlaybarStillProps) => {
  const source = useContext(PlaybarStillSourceContext);
  const resolved = uri ? source(uri) : undefined;
  return resolved ? (
    <Image
      accessibilityIgnoresInvertColors
      resizeMode="cover"
      source={resolved}
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
};

/** Supplies the stills' image sources to the player chrome under it. */
const PlaybarStillSourceProvider = PlaybarStillSourceContext.Provider;

export type { PlaybarStillProps, PlaybarStillSource };
export { PlaybarStill, PlaybarStillSourceProvider };
