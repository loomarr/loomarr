import * as React from "react";
import { cn } from "@/lib";

// A native <input type="checkbox"> styled with the Test Card tokens — the bool control
// for settings forms (config-design §2 kinds). Deliberately NOT Radix: shadcn's Checkbox
// would add @radix-ui/react-checkbox, and a native checkbox already gives correct
// keyboard + screen-reader semantics. `accent-signal` paints the checked state in brand.
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
