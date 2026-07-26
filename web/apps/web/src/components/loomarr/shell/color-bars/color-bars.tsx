import { cn } from "@/lib";
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

const ColorBars = ({ size = "lg", className }: ColorBarsProps) => (
  <div className={cn("flex overflow-hidden rounded-[2px]", HEIGHT[size], WIDTH[size], className)} aria-hidden>
    {SEGMENTS.map((bg) => (
      <span key={bg} className={cn("h-full flex-1", bg)} />
    ))}
  </div>
);

export { ColorBars };
