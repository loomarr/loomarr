import { mergeProps } from "@base-ui/react/merge-props";
import { useRender } from "@base-ui/react/use-render";
import { cn } from "@/lib/utils";
import type { ButtonProps, ButtonSize, ButtonVariant } from "./button.type";

// shadcn/ui Button (new-york), restyled via the Test Card tokens only — never fork
// the primitive logic (frontend-design §3, Layer 1). The signal focus ring is the
// brand focus treatment (§2.1).
//
// `class-variance-authority` is deliberately absent (#970 PR B checkpoint 3): `buttonVariants` is
// consumed directly as a className by 11 call sites outside `<Button>` (e.g.
// `className={buttonVariants({ variant: "outline", size: "sm" })}` on a plain `<a>`/`<Link>`), so
// its whole contract is "return a literal Tailwind class string" — there is no props channel for
// those call sites to also receive a style object, which is exactly why @loomarr/design-system's
// Action (a Tamagui view with no equivalent string-returning API) cannot back this component
// directly; see button.type.ts and the PR for the full gap list. `cva` itself added nothing this
// hand-written version does not: there were no `compoundVariants` here, so its output was always
// base + variant class + size class + caller className, in that order, which is what the
// ternaries below reproduce byte-for-byte through `cn` (kept inline, not hoisted to a lookup
// object the legacy-usage checker can't see through a dynamic index into — the ledger's own
// static analysis only counts a class literal it can find INSIDE a `cn`/`cva` call).
const buttonVariants = ({
  className,
  size,
  variant,
}: {
  className?: string;
  size?: ButtonSize | null;
  variant?: ButtonVariant | null;
} = {}) =>
  cn(
    "inline-flex cursor-pointer items-center justify-center gap-2 whitespace-nowrap rounded-md font-medium text-sm transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background disabled:pointer-events-none disabled:opacity-50 forced-colors:border forced-colors:border-current [&_svg]:size-4 [&_svg]:shrink-0",
    variant === "suggest"
      ? // The AI action (§2.1: suggest is "THE AI color"). Reserved for generation —
        // the Suggest button — so magenta reads as "this asks the model", distinct from
        // the amber primary. DARK text: white on solid suggest is only 4.12:1 (fails AA
        // for <18px), while static-950 clears it at 4.75:1 — the same reason the amber
        // primary carries dark text. (§2.1's calibration is stricter than the prototype's
        // white-on-magenta; the a11y gate enforces it.)
        "bg-suggest text-static-950 hover:bg-suggest/90"
      : variant === "destructive"
        ? // DARK text on the onair red, same AA calibration as `suggest`: white on solid
          // onair (#E5484D) is only 3.91:1 (fails AA for <18px — caught by the a11y gate on
          // the danger zone), while static-950 clears it comfortably.
          "bg-destructive text-static-950 hover:bg-destructive/90"
        : variant === "outline"
          ? "border border-input bg-transparent hover:bg-accent hover:text-accent-foreground"
          : variant === "secondary"
            ? "bg-secondary text-secondary-foreground hover:bg-secondary/80"
            : variant === "ghost"
              ? "hover:bg-accent hover:text-accent-foreground"
              : variant === "link"
                ? "text-tune underline-offset-4 hover:underline"
                : "bg-primary text-primary-foreground hover:bg-primary/90",
    size === "sm"
      ? "h-8 rounded-md px-3 text-xs"
      : size === "lg"
        ? "h-10 rounded-md px-6"
        : size === "icon"
          ? "h-9 w-9"
          : "h-9 px-4 py-2",
    className,
  );

// No `forwardRef`: on React 19 `ref` is an ordinary prop, and `useRender` merges it onto whatever
// `render` produces. `mergeProps` (not a plain spread) is what makes composition safe — it CHAINS
// event handlers and CONCATENATES className instead of letting the outer props clobber the inner
// ones, so `render={<Link onClick={…} className="…" />}` keeps both the Link's behaviour and the
// button's styling.
const Button = ({ className, variant, size, render, ...props }: ButtonProps) =>
  useRender({
    defaultTagName: "button",
    render,
    props: mergeProps<"button">({ className: cn(buttonVariants({ variant, size, className })) }, props),
  });

export { Button, buttonVariants };
