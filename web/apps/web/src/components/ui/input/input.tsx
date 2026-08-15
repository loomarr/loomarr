import { Eye, EyeOff } from "lucide-react";
import * as React from "react";
import { cn } from "@/lib/utils";

// shadcn/ui Input (new-york). The boundary is border-input (border-control ≥3:1,
// §2.3) plus the signal focus ring — a control is never identified by a hairline
// alone (WCAG 1.4.11). Typed text is text-foreground (crisp, not the ambient dim
// inherit), with a signal caret + signal text-selection so editing reads on-brand.
const baseClasses =
  "flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm text-foreground caret-signal shadow-sm transition-colors placeholder:text-muted-foreground selection:bg-signal/30 selection:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background disabled:cursor-not-allowed disabled:opacity-50";

// A masked password gets larger, wider-tracked dots so the value is legible at a glance;
// revealed text reverts to normal metrics so it reads like any other field. Both leave room
// (pr-9) for the reveal toggle.
const maskedClasses = "text-base tracking-[0.25em]";

const Input = React.forwardRef<HTMLInputElement, React.InputHTMLAttributes<HTMLInputElement>>(
  ({ className, type, ...props }, ref) => {
    // A password field carries a show/hide toggle: local reveal state flips the rendered
    // type between "password" and "text". Every other type renders exactly as before.
    const isPassword = type === "password";
    const [revealed, setRevealed] = React.useState(false);

    if (!isPassword) {
      return <input type={type} ref={ref} className={cn(baseClasses, className)} {...props} />;
    }

    return (
      <div className="relative">
        <input
          type={revealed ? "text" : "password"}
          ref={ref}
          className={cn(baseClasses, "pr-9", !revealed && maskedClasses, className)}
          {...props}
        />
        <button
          type="button"
          // Keyboard-reachable and labeled so it's usable by everyone; the label flips with
          // state so assistive tech announces the current action.
          aria-label={revealed ? "Hide password" : "Show password"}
          aria-pressed={revealed}
          onClick={() => setRevealed((v) => !v)}
          className="absolute inset-y-0 right-0 flex w-9 items-center justify-center rounded-r-md text-muted-foreground transition-colors hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background"
        >
          {revealed ? <EyeOff className="size-4" aria-hidden /> : <Eye className="size-4" aria-hidden />}
        </button>
      </div>
    );
  },
);
Input.displayName = "Input";

export { Input };
