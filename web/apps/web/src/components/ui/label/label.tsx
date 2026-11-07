import * as React from "react";
import { cn } from "@/lib/utils";

// A plain native <label>. Pair it with the control's id (`htmlFor`) for an accessible name, and a
// click on it focuses or toggles that control, as the browser does for any label.
//
// Not the design-system's `Text as="label"` (#970 step 45): every class below is an override
// target — 13 callers swap the colour and size for `text-muted-foreground text-xs`, others the
// weight for `font-normal` — and Text's `as` path writes colour, size and weight as inline style,
// which no caller class could then replace. The same call-site-verified exception Caption records.
// ⚠ That includes `leading-none`: a caller's size class (`text-xs`) carries its own line-height,
// and tailwind-merge drops `leading-none` for it. An inline line-height would survive that merge
// and shrink every such label by 4px.
const Label = React.forwardRef<HTMLLabelElement, React.LabelHTMLAttributes<HTMLLabelElement>>(
  ({ className, ...props }, ref) => (
    // biome-ignore lint/a11y/noLabelWithoutControl: this primitive receives htmlFor at each form call site; the linter cannot see through the prop spread.
    <label
      ref={ref}
      className={cn("text-foreground text-sm leading-none font-medium", className)}
      {...props}
    />
  ),
);
Label.displayName = "Label";

export { Label };
