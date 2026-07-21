import { cn } from "@/lib";
import type { TvStaticProps } from "./tv-static.type";

// TvStatic — the CRT snow behind idle surfaces (login, wizard, empty states) and NOWHERE
// near data, tables, or forms (§1). A full-bleed fractal-noise layer that FLICKERS like
// real TV static: the noise tile is oversized and its transform jumps between offsets on a
// stepped keyframe (`animate-tv-snow`, defined in the token theme), so the grain shifts a
// few times a second instead of sitting dead — a still render just reads as flat texture.
//
// Gated behind `motion-safe:` — the whole layer is `hidden` and only reappears when motion
// is allowed. That is ALSO the signal the visual suite pins (`reducedMotion: "reduce"`,
// playwright.shared), so baselines never rasterize GPU-dependent, animating noise and stay
// deterministic (§5.2, §17). Inert regardless: aria-hidden + pointer-events-none.
const TvStatic = ({ className }: TvStaticProps) => (
  <div
    className={cn("pointer-events-none absolute inset-0 hidden overflow-hidden motion-safe:block", className)}
    aria-hidden
  >
    {/* Oversized so the animated transform never exposes an edge; opacity lives here so
        the flicker rides a steady 9% wash. */}
    <svg
      className="absolute inset-[-50%] size-[200%] opacity-[0.09] motion-safe:animate-tv-snow"
      xmlns="http://www.w3.org/2000/svg"
    >
      <title>TV static</title>
      <filter id="loomarr-tv-static">
        <feTurbulence type="fractalNoise" baseFrequency="0.9" numOctaves={2} stitchTiles="stitch" />
      </filter>
      <rect width="100%" height="100%" filter="url(#loomarr-tv-static)" />
    </svg>
  </div>
);

export { TvStatic };
