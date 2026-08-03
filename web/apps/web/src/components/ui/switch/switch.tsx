import * as React from "react";
import { cn } from "@/lib";

// Switch — the pill toggle for "is this thing on?" (the v2 mock's source switches).
//
// Measured off the rendered mock rather than approximated: a 34×19px track at `rounded-full` with
// a 1px border, holding a 13px round knob inset 2px, which travels 2px → 16px. ON is a `lock`
// tint (22% fill, 50% border) with a solid `lock` knob; OFF is `background` with a `static-700`
// border and a `static-500` knob.
//
// ⚠ **A native `<input type="checkbox" role="switch">`, NOT a `<button>`.** The mock draws a
// button, but a button means hand-rolling `aria-checked`, space/enter activation and form
// participation — three things the native input already gets right. `role="switch"` is what turns
// the checkbox's "checked" into "on/off" for a screen reader. Same trade-off the Checkbox
// primitive records: correct semantics beat matching the mock's tag name, and nothing about the
// rendered pixels depends on it.
//
// ⚠ **`peer-checked:` drives the visuals, so the DOM state and the paint cannot disagree.** An
// earlier version of this control was a plain `accent-signal` checkbox — a small square where the
// mock has a pill, in brand amber where the mock uses `lock` green. Green is right here and the
// distinction is not decorative: amber is Loomarr's "this is the thing you are looking at" colour,
// while a source being ON is a *healthy* state, which is what `lock` means everywhere else.
const Switch = React.forwardRef<HTMLInputElement, React.InputHTMLAttributes<HTMLInputElement>>(
  ({ className, checked, ...props }, ref) => (
    // The label wraps both so a click anywhere on the pill toggles it, and so the input can stay
    // visually hidden while remaining the real focus target.
    <span className={cn("group relative inline-flex shrink-0 items-center", className)}>
      <input
        type="checkbox"
        role="switch"
        ref={ref}
        checked={checked}
        // ⚠ Explicit, even though the native `checked` above already conveys it. Declaring
        // `role="switch"` REPLACES the implicit checkbox semantics, so the state has to be
        // re-declared in ARIA terms or the control announces no state at all. Caught by the a11y
        // lint rather than by looking at it — and it is the same finding the filler row's old
        // hand-rolled checkbox had recorded, which is an argument for this being a primitive.
        aria-checked={checked}
        // `sr-only` rather than `hidden` or `opacity-0`: it must stay focusable and hit-testable,
        // which `display:none` would destroy.
        //
        // ⚠ It also carries the CURSOR and covers the whole pill, so the hit target is the track
        // rather than a 1px-tall invisible input. Without `inset-0` the operator can see a switch
        // and click straight through it.
        className="peer absolute inset-0 z-10 size-full cursor-pointer opacity-0 disabled:cursor-not-allowed"
        {...props}
      />
      {/* The track. `peer-focus-visible` puts the focus ring here because the input itself is
          visually hidden — without it, tabbing to this control shows nothing at all. */}
      <span
        aria-hidden
        className={cn(
          "pointer-events-none h-[19px] w-[34px] rounded-full border transition-colors",
          "border-static-700 bg-background",
          "peer-checked:border-lock/50 peer-checked:bg-lock/20",
          // ⚠ Hover brightens the BORDER to `static-400`. That is the mock's own dominant hover
          // language — 34 of its 68 hover rules are `border-color:#8B93A3` — rather than a
          // convention invented here. The mock does not style this particular toggle on hover at
          // all (it only sets `cursor:pointer`), so an affordance was added deliberately: a
          // control that changes state on click should say so before it is clicked.
          // ⚠ Applied to the checked state too, with the green kept — otherwise hovering an ON
          // switch would read as it turning off.
          "peer-hover:border-static-400 peer-checked:peer-hover:border-lock",
          "peer-focus-visible:ring-2 peer-focus-visible:ring-ring peer-focus-visible:ring-offset-2 peer-focus-visible:ring-offset-background",
          "peer-disabled:cursor-not-allowed peer-disabled:opacity-50",
          // A disabled switch must not offer the hover affordance — it cannot be flipped.
          "peer-disabled:peer-hover:border-static-700",
        )}
      />
      {/* The knob, positioned over the track. ⚠ `motion-safe:` on the travel — a reduced-motion
          user gets the same two positions without the slide. */}
      <span
        aria-hidden
        className={cn(
          "pointer-events-none absolute top-1/2 left-0.5 size-[13px] -translate-y-1/2 rounded-full",
          "bg-static-500 transition-transform motion-reduce:transition-none",
          "peer-checked:translate-x-3.5 peer-checked:bg-lock",
        )}
      />
    </span>
  ),
);
Switch.displayName = "Switch";

export { Switch };
