/// <reference lib="dom" />
import { Tooltip as TooltipPrimitive } from "@base-ui/react/tooltip";

import { semanticRadius } from "../tokens";
import type { TooltipProps } from "./tooltip.type";

const shadow = "0 4px 6px -1px rgb(0 0 0 / 0.1), 0 2px 4px -2px rgb(0 0 0 / 0.1)";

// The web adapter, on Base UI Tooltip: it opens on hover and keyboard focus, closes on Escape, and
// the trigger is the caller's own element, so its name and handlers stay where they were written.
// The surface reads the theme's CSS variables so light and dark both hold.
const Tooltip = ({ align, children, content, id, side, width }: TooltipProps) => (
  <TooltipPrimitive.Root>
    <TooltipPrimitive.Trigger delay={300} render={children} />
    <TooltipPrimitive.Portal>
      <TooltipPrimitive.Positioner align={align} side={side} sideOffset={6}>
        <TooltipPrimitive.Popup
          id={id}
          role="tooltip"
          style={{
            backgroundColor: "var(--surfaceRaised)",
            border: "1px solid var(--borderDecorative)",
            borderRadius: semanticRadius.control,
            boxShadow: shadow,
            color: "var(--contentPrimary)",
            fontSize: 12,
            lineHeight: "16px",
            maxWidth: width ?? 320,
            padding: width ? 12 : "6px 10px",
            width,
            zIndex: 50,
          }}
        >
          {content}
        </TooltipPrimitive.Popup>
      </TooltipPrimitive.Positioner>
    </TooltipPrimitive.Portal>
  </TooltipPrimitive.Root>
);

export { Tooltip };
