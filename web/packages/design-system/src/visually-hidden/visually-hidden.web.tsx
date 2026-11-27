/// <reference lib="dom" />
import type { CSSProperties } from "react";

import type { VisuallyHiddenProps } from "./visually-hidden.type";

// The web adapter: the standard clip-to-a-pixel recipe, not `display: none` or `visibility: hidden`,
// which would take the words out of the accessibility tree along with the eye.
const hidden: CSSProperties = {
  border: 0,
  clip: "rect(0 0 0 0)",
  height: 1,
  margin: -1,
  overflow: "hidden",
  padding: 0,
  position: "absolute",
  whiteSpace: "nowrap",
  width: 1,
};

const VisuallyHidden = ({ as: Host = "div", children }: VisuallyHiddenProps) => (
  <Host style={hidden}>{children}</Host>
);

export { VisuallyHidden };
