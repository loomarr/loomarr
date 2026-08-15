import * as React from "react";
import { cn } from "@/lib";

// shadcn/ui Label (new-york), plain <label> — no Radix dep needed for the simple
// case. Pair with the input's id for an accessible name.
const Label = React.forwardRef<HTMLLabelElement, React.LabelHTMLAttributes<HTMLLabelElement>>(
  ({ className, ...props }, ref) => (
    // biome-ignore lint/a11y/noLabelWithoutControl: this primitive receives htmlFor at each form call site; the linter cannot see through the prop spread.
    <label
      ref={ref}
      className={cn("font-medium text-foreground text-sm leading-none", className)}
      {...props}
    />
  ),
);
Label.displayName = "Label";

export { Label };
