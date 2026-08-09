import { Slider as SliderPrimitive } from "@base-ui/react/slider";
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
    {/* The primitive owns the slider's WAI-ARIA + keyboard contract. Styled to the mock (1010-1013):
        a translucent-white track, a WHITE fill, and an ALWAYS-VISIBLE white dot — on the dark video
        scrim a hover-only thumb + dim track reads as "no slider", which is exactly what happened.

        ⚠ Base UI takes a PLAIN NUMBER where Radix took a one-element array, and adds a `Control`
        element between Root and Track (Radix had none). `Range` is `Indicator` here. */}
    <SliderPrimitive.Root
      value={muted ? 0 : volume}
      max={1}
      step={0.05}
      onValueChange={(v) => {
        const next = typeof v === "number" ? v : (v[0] ?? 0);
        onVolumeChange(next);
        onMutedChange(next === 0);
      }}
      className="relative flex h-4 w-20 shrink-0 cursor-pointer touch-none select-none items-center"
    >
      <SliderPrimitive.Control className="flex w-full items-center">
        <SliderPrimitive.Track className="relative h-1 w-full grow rounded-full bg-static-0/25">
          <SliderPrimitive.Indicator className="absolute h-full rounded-full bg-static-0" />
          {/* ⚠ aria-label goes on the THUMB, not the Root. Under Radix that was because Radix put
              role="slider" on the thumb; under Base UI it is because each thumb renders its own
              nested <input type="range">, which is what carries the role. Same placement, different
              reason — re-verified against Base UI rather than copied forward. */}
          <SliderPrimitive.Thumb
            aria-label="Volume"
            className="block size-2.5 rounded-full bg-static-0 shadow transition-transform hover:scale-125 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-signal motion-reduce:transition-none"
          />
        </SliderPrimitive.Track>
      </SliderPrimitive.Control>
    </SliderPrimitive.Root>
  </div>
);

export { VolumeControl };
