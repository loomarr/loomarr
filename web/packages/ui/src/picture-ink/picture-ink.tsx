import { playerChrome, Surface, ThemeScope } from "@loomarr/design-system";
import type { ReactNode } from "react";
import { Platform } from "react-native";

/** How far the scrim feathers past what stands on it. */
const FEATHER = 64;
/** The side scrim's reach above and below its words, where it fades out. */
const SOFT = 32;

const ink = (toward: "right" | "top") =>
  `linear-gradient(to ${toward}, ${playerChrome.tooltip} calc(100% - ${FEATHER}px), transparent)`;

// The web feathers the ink away; a native client has no CSS gradient and draws it solid.
const scrim = (edge: "foot" | "side") => {
  if (Platform.OS !== "web") return { backgroundColor: playerChrome.scrimFoot };
  if (edge === "foot") return { backgroundImage: ink("top") };
  // The mask takes the hard top and bottom off, so the words stand on no box.
  const fade = `linear-gradient(transparent, black ${SOFT}px, black calc(100% - ${SOFT}px), transparent)`;
  return { backgroundImage: ink("right"), maskImage: fade };
};

/**
 * Chrome set over the picture: dark in both themes, on an ink scrim as wide as what stands on it
 * (`side`) or along the foot (`foot`).
 */
const PictureInk = ({ children, edge }: { children: ReactNode; edge: "foot" | "side" }) => {
  const side = edge === "side";
  const reach = side && Platform.OS === "web" ? -SOFT : 0;
  return (
    <ThemeScope theme="dark">
      <Surface
        alignSelf={side ? "flex-start" : "stretch"}
        backgroundColor="transparent"
        borderRadius={0}
        borderWidth={0}
        maxWidth="100%"
        paddingBottom={16}
        paddingLeft={side ? 0 : 16}
        paddingRight={side ? FEATHER : 16}
        paddingTop={side ? 16 : FEATHER}
        position="relative"
        testID={`picture-ink-${edge}`}
      >
        {/* The ink is a layer of its own, so its fade never touches the words. */}
        <Surface
          aria-hidden
          backgroundColor="transparent"
          borderRadius={0}
          borderWidth={0}
          bottom={reach}
          left={0}
          pointerEvents="none"
          position="absolute"
          right={0}
          top={reach}
          {...(scrim(edge) as object)}
        />
        <Surface backgroundColor="transparent" borderRadius={0} borderWidth={0} position="relative">
          {children}
        </Surface>
      </Surface>
    </ThemeScope>
  );
};

export { PictureInk };
