import { styled } from "@tamagui/core";

import { Surface } from "../primitives";

/** A raised surface with the compact 12px corners; callers lay out its content with props. */
const Card = styled(Surface, {
  name: "LoomarrCard",
  borderRadius: "cardCompact",
});

export { Card };
