import * as React from "react";
import { cn } from "@/lib/utils";

// A plain native <label>. Pair it with the control's id (`htmlFor`) for an accessible name, and a
// click on it focuses or toggles that control, as the browser does for any label.
//
// Not the design-system's `Text as="label"` (#970 step 45): every class below except the
// line-height is an override target — 13 callers swap the colour and size for
// `text-muted-foreground text-xs`, others the weight for `font-normal` — and Text's `as` path writes
// colour, size and weight as inline style, which no caller class could then replace. The same
// call-site-verified exception Caption records. The line-height, which no caller overrides, is
// inline.
const Label = React.forwardRef<HTMLLabelElement, React.LabelHTMLAttributes<HTMLLabelElement>>(
  ({ className, style, ...props }, ref) => (
    // biome-ignore lint/a11y/noLabelWithoutControl: this primitive receives htmlFor at each form call site; the linter cannot see through the prop spread.
    <label
      ref={ref}
      className={cn("font-medium text-foreground text-sm", className)}
      style={{ lineHeight: 1, ...style }}
      {...props}
    />
  ),
);
Label.displayName = "Label";

export { Label };
