import { Pause, Play } from "lucide-react";
import { Button } from "@/components/ui/button";
import type { PlayToggleProps } from "./play-toggle.type";

// PlayToggle — the player's play/pause button (§9.1 player controls). Amber (the app's action
// colour), a filled glyph. Its accessible name changes with STATE: a button permanently called
// "Play" lies the moment the video is playing, and a screen-reader user has no other cue.
//
// A named component, not inline JSX, so VideoPlayer's control bar COMPOSES its controls rather than
// hand-rolling each — matching the house pattern (its own folder, type, story, test).
const PlayToggle = ({ playing, onToggle }: PlayToggleProps) => (
  <Button
    variant="ghost"
    size="icon"
    onClick={onToggle}
    aria-label={playing ? "Pause" : "Play"}
    // hover grows the glyph, active dips it — a tactile press. motion-reduce disables it.
    className="text-signal hover:text-signal-400 size-8 shrink-0 rounded-full transition-transform hover:scale-110 hover:bg-transparent active:scale-95 motion-reduce:transition-none"
  >
    {playing ? <Pause className="fill-current" aria-hidden /> : <Play className="fill-current" aria-hidden />}
  </Button>
);

export { PlayToggle };
