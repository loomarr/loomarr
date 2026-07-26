import { cn } from "@/lib";
import type { OnAirIndicatorProps, OnAirState } from "./on-air-indicator.type";

// OnAirIndicator — the red dot (§3). `live` pulses (the one live-motion signature,
// §2.4) and is off under reduced-motion; `reconciling` is a steady caution dot;
// `off` is a muted dot. A sr-only sentence makes screen readers say "On air", not
// a bare glyph (§5.3).
const DOT: Record<OnAirState, string> = {
  off: "bg-static-500",
  live: "bg-onair",
  reconciling: "bg-caution",
};

const SR: Record<OnAirState, string> = {
  off: "Off air",
  live: "On air",
  reconciling: "Updating",
};

const OnAirIndicator = ({ state, showLabel = false, className }: OnAirIndicatorProps) => (
  <span className={cn("inline-flex items-center gap-1.5", className)}>
    <span className="relative flex size-2.5">
      {state === "live" && (
        <span
          className="absolute inline-flex size-full rounded-full bg-onair opacity-60 motion-safe:animate-ping"
          aria-hidden
        />
      )}
      <span className={cn("relative inline-flex size-2.5 rounded-full", DOT[state])} aria-hidden />
    </span>
    {showLabel && (
      <span
        className={cn(
          "font-mono text-2xs uppercase tracking-wide",
          state === "live" ? "text-onair-300" : "text-static-400",
        )}
      >
        {SR[state]}
      </span>
    )}
    <span className="sr-only">{SR[state]}</span>
  </span>
);

export { OnAirIndicator };
