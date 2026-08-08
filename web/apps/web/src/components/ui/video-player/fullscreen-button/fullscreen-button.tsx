import { Maximize, Minimize } from "lucide-react";
import { Button } from "@/components/ui/button";
import type { FullscreenButtonProps } from "./fullscreen-button.type";

// FullscreenButton — the icon-only fullscreen toggle at the far right of the control bar (§9.1
// Watch). Real OS fullscreen (the parent calls element.requestFullscreen), NOT an in-app dialog.
// Icon changes with state — enter vs exit — and the accessible name follows, so a screen-reader
// user knows which way it goes.
const FullscreenButton = ({ active, onToggle }: FullscreenButtonProps) => (
  <Button
    variant="ghost"
    size="icon"
    onClick={onToggle}
    aria-label={active ? "Exit fullscreen" : "Fullscreen"}
    className="size-8 shrink-0 rounded-full text-static-300 transition-transform hover:scale-110 hover:bg-transparent hover:text-static-0 active:scale-95 motion-reduce:transition-none"
  >
    {active ? <Minimize aria-hidden /> : <Maximize aria-hidden />}
  </Button>
);

export { FullscreenButton };
