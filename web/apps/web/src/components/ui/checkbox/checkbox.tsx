import * as React from "react";
import { cn } from "@/lib";

// A native <input type="checkbox"> styled with the Test Card tokens — the bool control
// for settings forms (config-design §2 kinds). Deliberately NOT a headless primitive:
// shadcn's Checkbox would pull one in, and a native checkbox already gives correct
// keyboard + screen-reader semantics. `accent-signal` paints the checked state in brand.
// (The rationale predates V50a and survives the vendor change — it was never about Radix
// specifically, only about not importing a primitive the platform already provides.)
const Checkbox = React.forwardRef<HTMLInputElement, React.InputHTMLAttributes<HTMLInputElement>>(
  ({ className, ...props }, ref) => (
    <input
      type="checkbox"
      ref={ref}
      className={cn(
        "size-4 shrink-0 cursor-pointer rounded-sm border border-input accent-signal transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background disabled:cursor-not-allowed disabled:opacity-50",
        className,
      )}
      {...props}
    />
  ),
);
Checkbox.displayName = "Checkbox";

export { Checkbox };
