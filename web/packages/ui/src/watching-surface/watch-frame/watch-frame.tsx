import { Surface, Text } from "@loomarr/design-system";
import { View } from "react-native";

import type { WatchFrameProps } from "./watch-frame.type";

// WatchFrame — the full-bleed page a web player owns (#2088 step 1, `/watch/$id`): the window's
// exact height, not at least it, so a picture told to fill has a definite height to fill (or it
// falls back to 16:9 and leaves a band). What doesn't fit, a portrait phone's panel, scrolls under
// the picture. Black in both themes: it is the picture's letterbox, not a theme surface.

// Off-screen but in the accessibility tree: the page still needs a heading to be reachable.
const visuallyHidden = {
  height: 1,
  margin: -1,
  overflow: "hidden",
  position: "absolute",
  width: 1,
} as const;

const WatchFrame = ({ children, title }: WatchFrameProps) => (
  <Surface
    backgroundColor="#000"
    borderRadius={0}
    borderWidth={0}
    flexDirection="column"
    height="100dvh"
    overflow="scroll"
    role="main"
  >
    <View style={visuallyHidden}>
      <Text as="h1" textRole="title">
        {title}
      </Text>
    </View>
    {children}
  </Surface>
);

export { WatchFrame };
