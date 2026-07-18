import * as React from "react";
import { cn } from "@/lib";

// shadcn/ui Label (new-york), plain <label> — no Radix dep needed for the simple
// case. Pair with the input's id for an accessible name.
const Label = React.forwardRef<HTMLLabelElement, React.LabelHTMLAttributes<HTMLLabelElement>>(
  ({ className, ...props }, ref) => (
    <label
      ref={ref}
      className={cn("font-medium text-foreground text-sm leading-none", className)}
      {...props}
    />
  ),
);
Label.displayName = "Label";

export { Label };
