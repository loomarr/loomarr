import type { Tooltip as TooltipPrimitive } from "@base-ui/react/tooltip";

// TooltipContent collapses three Base UI parts (Portal → Positioner → Popup) into the one
// component the app writes, so the positioning props have to be surfaced deliberately:
// `side`/`align`/`sideOffset` belong to the POSITIONER, everything else to the POPUP. Radix
// took all four on a single `Content`, which is why the split needs saying out loud here.
type TooltipContentProps = TooltipPrimitive.Popup.Props &
  Pick<TooltipPrimitive.Positioner.Props, "side" | "align" | "sideOffset" | "alignOffset">;

export type { TooltipContentProps };
