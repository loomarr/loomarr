import { Slot } from "@radix-ui/react-slot";
import { cva } from "class-variance-authority";
import * as React from "react";
import { cn } from "@/lib";
import type { ButtonProps } from "./button.type";

// shadcn/ui Button (new-york), restyled via the Test Card tokens only — never fork
// the primitive logic (frontend-design §3, Layer 1). The signal focus ring is the
// brand focus treatment (§2.1).
const buttonVariants = cva(
  "inline-flex items-center justify-center gap-2 whitespace-nowrap rounded-md font-medium text-sm transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background disabled:pointer-events-none disabled:opacity-50 [&_svg]:size-4 [&_svg]:shrink-0",
  {
    variants: {
      variant: {
        default: "bg-primary text-primary-foreground hover:bg-primary/90",
        // The AI action (§2.1: suggest is "THE AI color"). Reserved for generation —
        // the Suggest button — so magenta reads as "this asks the model", distinct from
        // the amber primary. DARK text: white on solid suggest is only 4.12:1 (fails AA
        // for <18px), while static-950 clears it at 4.75:1 — the same reason the amber
        // primary carries dark text. (§2.1's calibration is stricter than the prototype's
        // white-on-magenta; the a11y gate enforces it.)
        suggest: "bg-suggest text-static-950 hover:bg-suggest/90",
        destructive: "bg-destructive text-destructive-foreground hover:bg-destructive/90",
        outline: "border border-input bg-transparent hover:bg-accent hover:text-accent-foreground",
        secondary: "bg-secondary text-secondary-foreground hover:bg-secondary/80",
        ghost: "hover:bg-accent hover:text-accent-foreground",
        link: "text-tune underline-offset-4 hover:underline",
      },
      size: {
        default: "h-9 px-4 py-2",
        sm: "h-8 rounded-md px-3 text-xs",
        lg: "h-10 rounded-md px-6",
        icon: "h-9 w-9",
      },
    },
    defaultVariants: { variant: "default", size: "default" },
  },
);

const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant, size, asChild = false, ...props }, ref) => {
    const Comp = asChild ? Slot : "button";
    return <Comp className={cn(buttonVariants({ variant, size, className }))} ref={ref} {...props} />;
  },
);
Button.displayName = "Button";

export { Button, buttonVariants };
