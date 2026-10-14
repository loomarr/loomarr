import { cn } from "@/lib/utils";
import type { ColorBarsProps, ColorBarsSize } from "./color-bars.type";

// ColorBars — the SMPTE test-card strip that is the whole design's namesake (§1). Seven
// hard-edged segments in the broadcast accents, from the same tokens the UI uses for
// state, so the brand mark IS the palette: signal · caution · lock · tune · suggest ·
// onair · static-400. Purely decorative (aria-hidden); it names nothing.
const SEGMENTS = [
  "bg-signal",
  "bg-caution",
  "bg-lock",
  "bg-tune",
  "bg-suggest",
  "bg-onair",
  "bg-static-400",
] as const;

const HEIGHT: Record<ColorBarsSize, string> = {
  sm: "h-1.5", // ~6px — the sidebar lockup
  lg: "h-3.5", // 14px — the login/wizard hero (matches the prototype)
};

const WIDTH: Record<ColorBarsSize, string> = {
  sm: "w-14", // ~56px
  lg: "w-50", // 200px — the prototype's hero width
};

// How far apart the segments' breathe cycles are staggered. 0.28s across seven bars spreads
// the dip over ~1.7s of the 4s cycle, so the shimmer travels across the card instead of the
// whole strip pulsing at once — which is the difference between "live signal" and "spinner".
const BREATHE_STAGGER_S = 0.28;

const ColorBars = ({ size = "lg", breathe = false, className }: ColorBarsProps) => (
  <div className={cn("flex overflow-hidden rounded-[2px]", HEIGHT[size], WIDTH[size], className)} aria-hidden>
    {SEGMENTS.map((bg, i) => (
      <span
        key={bg}
        className={cn("h-full flex-1", bg, breathe && "motion-safe:animate-bar-breathe")}
        // Inline because the delay is per-INDEX: a utility class per segment would mean
        // seven arbitrary classnames that Tailwind must be told to keep, to express one
        // multiplication. Only set when breathing, so the still cases stay attribute-free.
        style={breathe ? { animationDelay: `${i * BREATHE_STAGGER_S}s` } : undefined}
      />
    ))}
  </div>
);

export { ColorBars };
