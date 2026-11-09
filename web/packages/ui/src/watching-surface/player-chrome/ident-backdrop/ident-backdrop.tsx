import { playerChrome } from "@loomarr/design-system";
import { Image, PixelRatio, View } from "react-native";

import { type PlayerArt, rungFor } from "../art";
import { type Frame, pictureIn } from "../watermark-clearance";

// The station-ID card's ground (§9.2): the programme's fanart, heavily dimmed, standing in for the
// video. It is drawn contained at 16:9 inside the frame (letterboxed like the video it replaces), never
// stretched to the screen's shape, from the rung the screen will actually show: a phone gets a small
// one, a 2x laptop a sharp one (#2774).

/** How much of the fanart shows: it is a ground for the card, not a picture. */
const DIM = 0.28;

const IdentBackdrop = ({ art, frame }: { art: PlayerArt; frame: Frame }) => {
  const picture = pictureIn(frame);
  return (
    <View
      accessibilityElementsHidden
      importantForAccessibility="no-hide-descendants"
      pointerEvents="none"
      style={{
        backgroundColor: playerChrome.surface,
        bottom: 0,
        left: 0,
        position: "absolute",
        right: 0,
        top: 0,
        zIndex: 29,
      }}
    >
      <Image
        resizeMode="contain"
        source={{ uri: rungFor(art, picture.width, PixelRatio.get()) }}
        style={{
          height: picture.height,
          left: picture.x,
          opacity: DIM,
          position: "absolute",
          top: picture.y,
          width: picture.width,
        }}
      />
    </View>
  );
};

export { IdentBackdrop };
