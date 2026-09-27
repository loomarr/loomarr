import { cn } from "@/lib/utils";
import type { TvStaticProps } from "./tv-static.type";

// TvStatic — the CRT snow behind idle surfaces (login, wizard, empty states) and NOWHERE
// near data, tables, or forms (§1). A full-bleed noise layer that FLICKERS like real TV
// static: the noise tile is oversized and its transform jumps between offsets on a stepped
// keyframe (`animate-tv-snow`, defined in the token theme), so the grain shifts a few times a
// second instead of sitting dead — a still render just reads as flat texture.
//
// The snow is the analog snow the maintainer approved for the docs art ("E1", #1620): sharp,
// full-greyscale speckle — fractal noise stretched taller than wide (1.6 × 2.8), pushed through
// a hard gain (×2.6, −0.82) so it snaps between black and white — with faint horizontal streaks
// (a second, very wide noise) composited over it, the way a set's snow smears along the lines.
// Seeded, so a still frame is the same every render.
//
// Two strengths of the one snow:
// - `idle` (default): a faint veil (~11%) with a whisper of scanlines. Gated behind
//   `motion-safe:` — the whole layer is `hidden` and only reappears when motion is allowed. That
//   is ALSO the signal the visual suite pins (`reducedMotion: "reduce"`, playwright.shared), so
//   idle baselines never rasterize noise (§5.2, §17).
// - `wash`: the tuner's channel-switch snow. It is what the viewer lands on INSTEAD of a picture,
//   so it is present under reduced motion (only the flicker stops). The snow is opaque, under a
//   static-950 layer at 90% and firmer scanlines, so the readout over it stays legible.
// Inert regardless: aria-hidden + pointer-events-none.

// The docs figure draws E1 in a 200-unit viewBox shown ~550 px wide, so its grain is ~2.75× the
// filter's raw per-pixel frequency. Drawing the noise at that scale keeps the grain the maintainer
// approved, rather than a far finer one.
const GRAIN_SCALE = 2.75;

const AnalogSnow = ({ id, className }: { id: string; className: string }) => (
  <svg className={cn("absolute inset-[-50%] size-[200%]", className)} xmlns="http://www.w3.org/2000/svg">
    <title>TV static</title>
    <filter id={id} x="0" y="0" width="100%" height="100%" colorInterpolationFilters="sRGB">
      <feTurbulence type="fractalNoise" baseFrequency="1.6 2.8" numOctaves={2} seed={4} result="grain" />
      <feColorMatrix
        in="grain"
        type="matrix"
        values="2.6 0 0 0 -0.82  2.6 0 0 0 -0.82  2.6 0 0 0 -0.8  0 0 0 0 1"
        result="snow"
      />
      <feTurbulence type="fractalNoise" baseFrequency="0.004 0.9" numOctaves={1} seed={11} result="lines" />
      <feColorMatrix
        in="lines"
        type="matrix"
        values="0 0 0 0 1  0 0 0 0 1  0 0 0 0 1  1.4 0 0 0 -0.62"
        result="streak"
      />
      <feComposite in="streak" in2="snow" operator="over" />
    </filter>
    <g transform={`scale(${GRAIN_SCALE})`}>
      <rect
        width={`${100 / GRAIN_SCALE}%`}
        height={`${100 / GRAIN_SCALE}%`}
        filter={`url(#${id})`}
        shapeRendering="crispEdges"
      />
    </g>
  </svg>
);

const TvStatic = ({ variant = "idle", className }: TvStaticProps) =>
  variant === "wash" ? (
    <div
      data-snow="wash"
      className={cn("pointer-events-none absolute inset-0 overflow-hidden", className)}
      aria-hidden
    >
      <AnalogSnow id="loomarr-tv-static-wash" className="motion-safe:animate-tv-snow" />
      {/* The dark between the snow and the readout: static-950 at 90%, so the snow reads only faintly
          and the text on it stays crisp (the maintainer's pick from the demo). */}
      <div className="absolute inset-0 bg-static-950/90" />
      {/* Scanlines: a dark line every third pixel, holding still over the snow. */}
      <div
        className="absolute inset-0"
        style={{
          backgroundImage: "repeating-linear-gradient(0deg, rgba(0,0,0,0.18) 0 1px, transparent 1px 3px)",
        }}
      />
    </div>
  ) : (
    <div
      className={cn(
        "pointer-events-none absolute inset-0 hidden overflow-hidden motion-safe:block",
        className,
      )}
      aria-hidden
    >
      {/* Oversized so the animated transform never exposes an edge; opacity lives here so
          the flicker rides a steady, subtle wash (~11%). */}
      <AnalogSnow id="loomarr-tv-static" className="opacity-[0.11] motion-safe:animate-tv-snow" />
      {/* Subtle horizontal scanlines — a faint CRT read layered over the snow. Static (not
          animated): real scanlines hold still while the snow flickers beneath them. */}
      <div
        className="absolute inset-0 opacity-40"
        style={{
          backgroundImage:
            "repeating-linear-gradient(0deg, transparent 0px, transparent 2px, rgba(0,0,0,0.06) 3px, transparent 4px)",
        }}
      />
    </div>
  );

export { TvStatic };
