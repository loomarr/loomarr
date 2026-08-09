import { Tooltip as TooltipPrimitive } from "@base-ui/react/tooltip";
import { cn } from "@/lib";
import type { TooltipContentProps } from "./tooltip.type";

// Icon-only-button labels (design §14). The native `title=` attribute is unstyled,
// ~1s-delayed, and keyboard/touch-hostile; this is a themed Base UI tooltip matching the
// Select recipe — a `popover`-token surface with the same fade+rise entrance, so every
// icon affordance (sidebar, back arrow, row actions) can say what it does on hover/focus.
//
// TooltipProvider is mounted once at the app root (a single provider shares the
// open/close delay timers across all tooltips); individual tooltips need no provider of
// their own. ⚠ Its prop is `delay`, not Radix's `delayDuration` — both `__root.tsx` and
// `.storybook/preview.tsx` mount one, and a provider that silently ignores an unknown prop
// would leave the app and the workshop disagreeing about hover timing with nothing red.
//
// ⚠ Base UI splits Radix's single `Content` into Portal → Positioner → Popup. Positioning
// (`side`/`align`/`sideOffset`) belongs to the POSITIONER; the surface styling belongs to
// the POPUP. `TooltipContent` keeps the one-component call shape the 37 consumers already
// use and routes each prop to the part that owns it — see `tooltip.type.ts`.
const TooltipProvider = TooltipPrimitive.Provider;
const Tooltip = TooltipPrimitive.Root;
const TooltipTrigger = TooltipPrimitive.Trigger;

const TooltipContent = ({
  className,
  side,
  align,
  sideOffset = 6,
  alignOffset,
  children,
  ...props
}: TooltipContentProps) => (
  <TooltipPrimitive.Portal>
    <TooltipPrimitive.Positioner side={side} align={align} sideOffset={sideOffset} alignOffset={alignOffset}>
      <TooltipPrimitive.Popup
        className={cn(
          "z-50 max-w-xs rounded-md border border-border bg-popover px-2.5 py-1.5 text-popover-foreground text-xs shadow-md",
          // Fade + rise on open — off under reduced-motion (and frozen in the visual suite).
          "motion-safe:animate-select-in",
          className,
        )}
        {...props}
      >
        {children}
      </TooltipPrimitive.Popup>
    </TooltipPrimitive.Positioner>
  </TooltipPrimitive.Portal>
);

export { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger };
