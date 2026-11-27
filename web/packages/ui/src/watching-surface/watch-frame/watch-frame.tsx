import { Surface, Text, VisuallyHidden } from "@loomarr/design-system";

import type { WatchFrameProps } from "./watch-frame.type";

// WatchFrame — the full-bleed page a web player owns (#2088 step 1, `/watch/$id`): the window's
// exact height, not at least it, so a picture told to fill has a definite height to fill (or it
// falls back to 16:9 and leaves a band). What doesn't fit, a portrait phone's panel, scrolls under
// the picture. Black in both themes: it is the picture's letterbox, not a theme surface.
//
// ⚠ It scrolls down only when something overflows (`auto`), and never sideways. `overflow: scroll`
// draws both scrollbars permanently wherever scrollbars are not overlays (Windows, Linux), which
// took 15px off the picture on each axis with nothing to scroll (#3405).

const WatchFrame = ({ children, title }: WatchFrameProps) => (
  <Surface
    backgroundColor="#000"
    borderRadius={0}
    borderWidth={0}
    flexDirection="column"
    height="100dvh"
    overflowX="hidden"
    overflowY="auto"
    role="main"
  >
    {/* Off-screen but in the accessibility tree: the page still needs a heading to be reachable. */}
    <VisuallyHidden>
      <Text as="h1" textRole="title">
        {title}
      </Text>
    </VisuallyHidden>
    {children}
  </Surface>
);

export { WatchFrame };
