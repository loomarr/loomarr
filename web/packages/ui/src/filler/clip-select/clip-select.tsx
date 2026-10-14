import { Toggle } from "@loomarr/design-system";

import type { ClipSelectProps } from "./clip-select.type";

// The Filler Library's bulk-select control (#1817, decided on #1908), shared by the clip card and the
// list row. A design-system checkbox at pointer density: a 24px box in a 40px-tall target, so it
// clears WCAG 2.2's 24px minimum (2.5.8). The label is announced, never printed: the card or row
// already shows the clip's name beside it.
const ClipSelect = ({ clipName, onSelectedChange, selected }: ClipSelectProps) => (
  <Toggle
    checked={selected}
    kind="checkbox"
    label={`Select ${clipName}`}
    labelHidden
    onCheckedChange={onSelectedChange}
  />
);

export { ClipSelect };
