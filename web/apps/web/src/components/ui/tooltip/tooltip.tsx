import * as TooltipPrimitive from "@radix-ui/react-tooltip";
import * as React from "react";
import { cn } from "@/lib";

// Icon-only-button labels (design §14). The native `title=` attribute is unstyled,
// ~1s-delayed, and keyboard/touch-hostile; this is a themed Radix tooltip matching the
// Select recipe — a `popover`-token surface with the same fade+rise entrance, so every
// icon affordance (sidebar, back arrow, row actions) can say what it does on hover/focus.
//
// TooltipProvider is mounted once at the app root (a single provider shares the
// open/close delay timers across all tooltips, per Radix); individual tooltips need no
// provider of their own.
const TooltipProvider = TooltipPrimitive.Provider;
const Tooltip = TooltipPrimitive.Root;
const TooltipTrigger = TooltipPrimitive.Trigger;

const TooltipContent = React.forwardRef<
  React.ElementRef<typeof TooltipPrimitive.Content>,
  React.ComponentPropsWithoutRef<typeof TooltipPrimitive.Content>
>(({ className, sideOffset = 6, children, ...props }, ref) => (
  <TooltipPrimitive.Portal>
    <TooltipPrimitive.Content
      ref={ref}
      sideOffset={sideOffset}
      className={cn(
        "z-50 max-w-xs rounded-md border border-border bg-popover px-2.5 py-1.5 text-popover-foreground text-xs shadow-md",
        // Fade + rise on open — off under reduced-motion (and frozen in the visual suite).
        "motion-safe:animate-select-in",
        className,
      )}
      {...props}
    >
      {children}
    </TooltipPrimitive.Content>
  </TooltipPrimitive.Portal>
));
TooltipContent.displayName = TooltipPrimitive.Content.displayName;

export { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger };
