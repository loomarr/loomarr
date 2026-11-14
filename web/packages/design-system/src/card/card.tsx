import { styled } from "@tamagui/core";

import { Surface } from "../primitives";
import { tintFrame } from "../tokens";

/**
 * A raised surface with the compact 12px corners; callers lay out its content with props. `tone`
 * draws a tinted frame instead of the raised fill: a 40% hairline of the brand hue over a 5% wash
 * of it (`caution` for a warning, `signal` for the amber primary, `onair` for a failure, `suggest`
 * for the AI magenta, `tune` for the cyan). The hue is the same in both themes.
 */
const Card = styled(Surface, {
  name: "LoomarrCard",
  borderRadius: "cardCompact",
  variants: {
    tone: {
      caution: tintFrame.caution,
      onair: tintFrame.onair,
      signal: tintFrame.signal,
      suggest: tintFrame.suggest,
      tune: tintFrame.tune,
    },
  } as const,
});

export { Card };
