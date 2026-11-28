import { Scrim, Surface, ThemeScope } from "@loomarr/design-system";
import type { ReactNode } from "react";

/** How far the scrim feathers past what stands on it. */
const FEATHER = 64;
/** The side scrim's reach above and below its words, where it fades out. */
const SOFT = 32;

/**
 * Chrome set over the picture: dark in both themes, on an ink scrim as wide as what stands on it
 * (`side`) or along the foot (`foot`).
 */
const PictureInk = ({ children, edge }: { children: ReactNode; edge: "foot" | "side" }) => {
  const side = edge === "side";
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
        <Scrim feather={FEATHER} from={side ? "right" : "top"} soften={side ? SOFT : 0} />
        <Surface backgroundColor="transparent" borderRadius={0} borderWidth={0} position="relative">
          {children}
        </Surface>
      </Surface>
    </ThemeScope>
  );
};

export { PictureInk };
