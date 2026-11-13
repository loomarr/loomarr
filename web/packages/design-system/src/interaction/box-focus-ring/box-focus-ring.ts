import type { Density } from "../../tokens";

// The amber focus ring around a checkbox, switch or radio box, outside a gap so it never touches
// the box's own border.
const boxFocusRing = (focused: boolean, density: Density) =>
  focused
    ? ({
        outlineColor: "actionFocus",
        outlineOffset: 2,
        outlineStyle: "solid",
        outlineWidth: density === "tv" ? 3 : 2,
      } as const)
    : {};

export { boxFocusRing };
