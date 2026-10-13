import { semanticColors } from "@loomarr/design-system";
import * as React from "react";
import { cn } from "@/lib/utils";

// A native <input type="checkbox"> styled with the shared design-system tokens — the bool control
// for settings forms (config-design §2 kinds). Deliberately NOT a headless primitive, and not the
// design-system's universal Toggle either: Toggle is a pressable that announces a checkbox role but
// is no form control, so it has no `name`/`value` in a submitted form, no `required`, and no
// browser validation. A native checkbox already gives correct keyboard (Space) and screen-reader
// semantics. The brand accent paints the checked state.
// (The rationale predates V50a and survives the vendor change — it was never about Radix
// specifically, only about not importing a primitive the platform already provides.)
//
// Token-backed like the Input (#970 step 45): the accent and border come from `semanticColors` as
// inline style; the size, radius and the cursor (which `disabled:` flips) stay classes.
const Checkbox = React.forwardRef<HTMLInputElement, React.InputHTMLAttributes<HTMLInputElement>>(
  ({ className, style, ...props }, ref) => (
    <input
      type="checkbox"
      ref={ref}
      className={cn(
        "size-4 cursor-pointer rounded-sm transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background disabled:cursor-not-allowed disabled:opacity-50",
        className,
      )}
      style={{
        flexShrink: 0,
        borderWidth: 1,
        borderStyle: "solid",
        borderColor: semanticColors.border.control,
        accentColor: semanticColors.action.primary,
        ...style,
      }}
      {...props}
    />
  ),
);
Checkbox.displayName = "Checkbox";

export { Checkbox };
