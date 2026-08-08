import * as SliderPrimitive from "@radix-ui/react-slider";
import { Volume2, VolumeX } from "lucide-react";
import { Button } from "@/components/ui/button";
import type { VolumeControlProps } from "./volume-control.type";

// VolumeControl — a mute toggle beside a volume slider, the player's `vol [====]` (the mock's
// bottom bar). Muting is its OWN button because a click is faster than dragging to zero; dragging
// the level to a non-zero value un-mutes, which is what every player does. Controlled: the parent
// owns `volume` (0–1) and `muted` and mirrors them onto the media element.
//
// A named component, not inline JSX — the control bar composes it (its own folder/type/story/test).
const VolumeControl = ({ volume, muted, onVolumeChange, onMutedChange }: VolumeControlProps) => (
  <div className="flex shrink-0 items-center gap-1.5">
    <Button
      variant="ghost"
      size="icon"
      onClick={() => onMutedChange(!muted)}
      aria-label={muted ? "Unmute" : "Mute"}
      className="size-8 rounded-full text-static-300 transition-transform hover:scale-110 hover:bg-transparent hover:text-static-100 active:scale-95 motion-reduce:transition-none"
    >
      {muted ? <VolumeX aria-hidden /> : <Volume2 aria-hidden />}
    </Button>
    {/* Radix owns the slider's WAI-ARIA + keyboard contract. Styled to the mock (1010-1013): a
        translucent-white track, a WHITE fill, and an ALWAYS-VISIBLE white dot — on the dark video
        scrim a hover-only thumb + dim track reads as "no slider", which is exactly what happened. */}
    <SliderPrimitive.Root
      value={[muted ? 0 : volume]}
      max={1}
      step={0.05}
      onValueChange={([v]) => {
        const next = v ?? 0;
        onVolumeChange(next);
        onMutedChange(next === 0);
      }}
      className="relative flex h-4 w-20 cursor-pointer touch-none select-none items-center"
    >
      <SliderPrimitive.Track className="relative h-1 w-full grow rounded-full bg-static-0/25">
        <SliderPrimitive.Range className="absolute h-full rounded-full bg-static-0" />
      </SliderPrimitive.Track>
      {/* ⚠ aria-label goes on the THUMB, not the Root — Radix puts role="slider" on the thumb, so
          the name must live there or a screen reader (and getByRole) sees an unnamed slider. */}
      <SliderPrimitive.Thumb
        aria-label="Volume"
        className="block size-2.5 rounded-full bg-static-0 shadow transition-transform hover:scale-125 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-signal motion-reduce:transition-none"
      />
    </SliderPrimitive.Root>
  </div>
);

export { VolumeControl };
