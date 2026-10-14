import { semanticColors } from "@loomarr/design-system";
import * as React from "react";
import { cn } from "@/lib/utils";

// A native <input type="checkbox"> styled with the shared design-system tokens — the bool control
// for settings forms (config-design §2 kinds). Deliberately NOT a headless primitive, and not the
// design-system's universal Toggle either: Toggle is a pressable that announces a checkbox role but
// is no form control, so it has no `name`/`value` in a submitted form, no `required`, and no
// browser validation. A native checkbox already gives correct keyboard (Space) and screen-reader
// semantics.
// (The rationale predates V50a and survives the vendor change — it was never about Radix
// specifically, only about not importing a primitive the platform already provides.)
//
// Checked is a selection, so it is neutral-pressed like the design-system Toggle (#1909): elevated
// fill, foreground border and a foreground mark. Amber is only the focus ring. `accent-color` cannot
// draw that (the browser picks the mark's colour and ignores the border), so the box is
// `appearance-none` and paints its own mark; the element stays a real checkbox. The box is 24 px,
// the WCAG 2.5.8 target minimum.
//
// Token-backed like the Input (#970 step 45): the colours come from `semanticColors` as inline
// style and CSS variables; the size, radius and the cursor (which `disabled:` flips) stay classes.
const checkMark = `url("data:image/svg+xml,${encodeURIComponent(
  `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16" fill="none" stroke="${semanticColors.content.primary}" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M3.5 8.5l3 3 6-7"/></svg>`,
)}")`;

const Checkbox = React.forwardRef<HTMLInputElement, React.InputHTMLAttributes<HTMLInputElement>>(
  ({ className, style, ...props }, ref) => (
    <input
      type="checkbox"
      ref={ref}
      className={cn(
        "checked:bg-(image:--checkbox-mark) size-6 cursor-pointer appearance-none rounded-sm border border-(--checkbox-border) border-solid bg-background bg-center bg-no-repeat transition-colors checked:border-(--checkbox-checked-border) checked:bg-(--checkbox-checked-fill) focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background disabled:cursor-not-allowed disabled:opacity-50",
        className,
      )}
      style={
        {
          flexShrink: 0,
          "--checkbox-border": semanticColors.border.control,
          "--checkbox-checked-border": semanticColors.content.primary,
          "--checkbox-checked-fill": semanticColors.surface.elevated,
          "--checkbox-mark": checkMark,
          ...style,
        } as React.CSSProperties
      }
      {...props}
    />
  ),
);
Checkbox.displayName = "Checkbox";

export { Checkbox };
