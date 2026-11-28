import { playerChrome, Surface, ThemeScope } from "@loomarr/design-system";
import type { ReactNode } from "react";
import { Platform } from "react-native";

/** How far the scrim feathers past what stands on it. */
const FEATHER = 64;

// The web feathers the ink away; a native client has no CSS gradient and draws it solid.
const scrim = (toward: "right" | "top") =>
  Platform.OS === "web"
    ? {
        backgroundImage: `linear-gradient(to ${toward}, ${playerChrome.tooltip} calc(100% - ${FEATHER}px), transparent)`,
      }
    : { backgroundColor: playerChrome.scrimFoot };

/**
 * Chrome set over the picture: dark in both themes, on an ink scrim as wide as what stands on it
 * (`side`) or along the foot (`foot`).
 */
const PictureInk = ({ children, edge }: { children: ReactNode; edge: "foot" | "side" }) => (
  <ThemeScope theme="dark">
    <Surface
      alignSelf={edge === "side" ? "flex-start" : "stretch"}
      backgroundColor="transparent"
      borderRadius={0}
      borderWidth={0}
      maxWidth="100%"
      paddingBottom={16}
      paddingHorizontal={edge === "side" ? 0 : 16}
      paddingRight={edge === "side" ? FEATHER : 16}
      paddingTop={edge === "side" ? 16 : FEATHER}
      testID={`picture-ink-${edge}`}
      {...(scrim(edge === "side" ? "right" : "top") as object)}
    >
      {children}
    </Surface>
  </ThemeScope>
);

export { PictureInk };
