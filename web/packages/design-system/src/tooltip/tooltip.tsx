import type { TooltipProps } from "./tooltip.type";

// Touch screens and TV remotes have no hover, so the native adapter shows the trigger alone. The
// trigger already carries its accessible name, which is all a tooltip ever repeats.
const Tooltip = ({ children }: TooltipProps) => children;

export { Tooltip };
