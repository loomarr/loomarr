import * as React from "react";
import { cn } from "@/lib";

// A native <select> styled to match Input — the enum control for settings forms
// (config-design §2 KindEnum). Deliberately NOT Radix: shadcn's Select would add
// @radix-ui/react-select for a closed list of 2–5 options, where the native control is
// already accessible, keyboard-complete, and correct on mobile. Same border-input
// boundary + signal focus ring as Input, so a form reads as one system (§2.3).
const Select = React.forwardRef<HTMLSelectElement, React.SelectHTMLAttributes<HTMLSelectElement>>(
  ({ className, children, ...props }, ref) => (
    <select
      ref={ref}
      className={cn(
        "flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background disabled:cursor-not-allowed disabled:opacity-50",
        className,
      )}
      {...props}
    >
      {children}
    </select>
  ),
);
Select.displayName = "Select";

export { Select };
