import { semanticColors } from "@loomarr/design-system";
import { Eye, EyeOff } from "lucide-react";
import * as React from "react";
import { cn } from "@/lib/utils";

// The text field: a real <input>, so labels, `required`, `aria-invalid`, `aria-describedby`, form
// submission and the browser's own validation message all stay native (#970 step 45). The boundary
// is the control border (≥3:1, §2.3) plus the signal focus ring — a control is never identified by
// a hairline alone (WCAG 1.4.11). Typed text is the primary content colour (crisp, not the ambient
// dim inherit), with a signal caret + signal text-selection so editing reads on-brand.
//
// Backed on @loomarr/design-system's semantic tokens the way checkpoints 1-3 backed Badge, Caption
// and Card: the values no caller overrides are inline styles sourced from `semanticColors`. What
// stays a class is either a pseudo-state (placeholder, selection, focus ring, disabled) that an
// inline style cannot express, an override target a real caller replaces through tailwind-merge
// (`w-28`, `pl-9`, `h-20 text-3xl` on the pairing code), or a box-shadow: the focus ring IS a
// box-shadow, so an inline `shadow-sm` would erase it.
const baseClasses =
  "h-9 w-full rounded-md px-3 py-1 text-sm shadow-sm transition-colors placeholder:text-muted-foreground selection:bg-signal/30 selection:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background disabled:cursor-not-allowed disabled:opacity-50";

const baseStyle: React.CSSProperties = {
  display: "flex",
  borderWidth: 1,
  borderStyle: "solid",
  borderColor: semanticColors.border.control,
  backgroundColor: "transparent",
  color: semanticColors.content.primary,
  caretColor: semanticColors.action.primary,
};

// A masked password gets larger, wider-tracked dots so the value is legible at a glance;
// revealed text reverts to normal metrics so it reads like any other field. Both leave room
// (pr-9) for the reveal toggle.
const maskedClasses = "text-base tracking-[0.25em]";

const Input = React.forwardRef<HTMLInputElement, React.InputHTMLAttributes<HTMLInputElement>>(
  ({ className, type, style, ...props }, ref) => {
    // A password field carries a show/hide toggle: local reveal state flips the rendered
    // type between "password" and "text". Every other type renders exactly as before.
    const isPassword = type === "password";
    const [revealed, setRevealed] = React.useState(false);
    // A caller's own style still wins, as it did when every value here was a class.
    const fieldStyle = { ...baseStyle, ...style };

    if (!isPassword) {
      return (
        <input type={type} ref={ref} className={cn(baseClasses, className)} style={fieldStyle} {...props} />
      );
    }

    return (
      <div style={{ position: "relative" }}>
        <input
          type={revealed ? "text" : "password"}
          ref={ref}
          className={cn(baseClasses, "pr-9", !revealed && maskedClasses, className)}
          style={fieldStyle}
          {...props}
        />
        <button
          type="button"
          // Keyboard-reachable and labeled so it's usable by everyone; the label flips with
          // state so assistive tech announces the current action.
          aria-label={revealed ? "Hide password" : "Show password"}
          aria-pressed={revealed}
          onClick={() => setRevealed((v) => !v)}
          // The colour stays a class: hover brightens it, and an inline colour would pin it.
          className="rounded-r-md text-muted-foreground transition-colors hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background"
          style={{
            position: "absolute",
            top: 0,
            bottom: 0,
            right: 0,
            display: "flex",
            width: 36,
            alignItems: "center",
            justifyContent: "center",
          }}
        >
          {revealed ? <EyeOff className="size-4" aria-hidden /> : <Eye className="size-4" aria-hidden />}
        </button>
      </div>
    );
  },
);
Input.displayName = "Input";

export { Input };
