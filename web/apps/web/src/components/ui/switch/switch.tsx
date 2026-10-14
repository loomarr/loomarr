import * as React from "react";
import { cn } from "@/lib/utils";

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
//
// Not the design-system's universal `Toggle kind="switch"` (#970 step 45), for the same reason as
// the Checkbox: Toggle is a pressable with no form participation. The geometry no state changes is
// inline style; every colour stays a `peer-*` class, because it follows the input's live checked,
// hover, focus and disabled state, which an inline style cannot see.
const Switch = React.forwardRef<HTMLInputElement, React.InputHTMLAttributes<HTMLInputElement>>(
  ({ className, checked, style, ...props }, ref) => (
    // The label wraps both so a click anywhere on the pill toggles it, and so the input can stay
    // visually hidden while remaining the real focus target.
    <span
      className={className}
      style={{ position: "relative", display: "inline-flex", flexShrink: 0, alignItems: "center" }}
    >
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
        // `peer` is the marker the track and knob read state from; the cursor flips on `disabled:`.
        className="peer cursor-pointer disabled:cursor-not-allowed"
        {...props}
        style={{
          position: "absolute",
          inset: 0,
          zIndex: 10,
          width: "100%",
          height: "100%",
          opacity: 0,
          ...style,
        }}
      />
      {/* The track. `peer-focus-visible` puts the focus ring here because the input itself is
          visually hidden — without it, tabbing to this control shows nothing at all. */}
      <span
        aria-hidden
        style={{
          pointerEvents: "none",
          width: 34,
          height: 19,
          borderRadius: 9999,
          borderWidth: 1,
          borderStyle: "solid",
        }}
        className={cn(
          "transition-colors",
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
        // The vertical centring stays a translate class: the checked travel is a translate class
        // too, and both write the one CSS `translate` property through Tailwind's shared variables.
        style={{
          pointerEvents: "none",
          position: "absolute",
          top: "50%",
          left: 2,
          width: 13,
          height: 13,
          borderRadius: 9999,
        }}
        className={cn(
          "-translate-y-1/2",
          "bg-static-500 transition-transform motion-reduce:transition-none",
          "peer-checked:translate-x-3.5 peer-checked:bg-lock",
        )}
      />
    </span>
  ),
);
Switch.displayName = "Switch";

export { Switch };
