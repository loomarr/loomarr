import { cn } from "@/lib/utils";
import type { TvStaticProps } from "./tv-static.type";

// TvStatic — the CRT snow behind idle surfaces (login, wizard, empty states) and NOWHERE
// near data, tables, or forms (§1). A full-bleed fractal-noise layer that FLICKERS like
// real TV static: the noise tile is oversized and its transform jumps between offsets on a
// stepped keyframe (`animate-tv-snow`, defined in the token theme), so the grain shifts a
// few times a second instead of sitting dead — a still render just reads as flat texture.
//
// The filter is what sells "snow" vs the old grid-y look: raw fractalNoise at a low base
// frequency reads as a repeating cellular grid, so we (1) DESATURATE it to white
// (feColorMatrix saturate=0) and (2) push it through a hard alpha contrast
// (feComponentTransfer) — the smooth noise becomes a sparse field of sharp white specks on
// transparent, i.e. real TV snow. A faint scanline overlay adds the CRT read without the
// "broken monitor" heaviness.
//
// Gated behind `motion-safe:` — the whole layer is `hidden` and only reappears when motion
// is allowed. That is ALSO the signal the visual suite pins (`reducedMotion: "reduce"`,
// playwright.shared), so baselines never rasterize GPU-dependent, animating noise and stay
// deterministic (§5.2, §17). Inert regardless: aria-hidden + pointer-events-none.
//
// The `wash` variant is the tuner's channel-switch snow (#1620 B4), and it differs on purpose: it
// is what the viewer lands on INSTEAD of a picture, so it is dense (every pixel a grey level, not
// sparse specks), darker and lower-contrast, at half strength so the readout over it stays legible.
// Under reduced motion it is still snow rather than nothing — only the flicker stops. The noise is
// seeded, so that still frame is the same every render.
const TvStatic = ({ variant = "idle", className }: TvStaticProps) =>
  variant === "wash" ? (
    <div
      data-snow="wash"
      className={cn("pointer-events-none absolute inset-0 overflow-hidden", className)}
      aria-hidden
    >
      <svg
        className="absolute inset-[-50%] size-[200%] opacity-50 motion-safe:animate-tv-snow"
        xmlns="http://www.w3.org/2000/svg"
      >
        <title>TV static</title>
        <filter id="loomarr-tv-static-wash">
          <feTurbulence
            type="fractalNoise"
            baseFrequency="1.1"
            numOctaves={1}
            seed={2}
            stitchTiles="stitch"
          />
          {/* Opaque grey from the noise's alpha, pulled down (-0.34) so the field sits dark, with the
              blue channel a touch lifted, like a set's cold phosphor. */}
          <feColorMatrix
            type="matrix"
            values="0 0 0 1.2 -0.34  0 0 0 1.2 -0.34  0 0 0 1.2 -0.32  0 0 0 0 1"
          />
        </filter>
        <rect width="100%" height="100%" filter="url(#loomarr-tv-static-wash)" />
      </svg>
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
      <svg
        className="absolute inset-[-50%] size-[200%] opacity-[0.11] motion-safe:animate-tv-snow"
        xmlns="http://www.w3.org/2000/svg"
      >
        <title>TV static</title>
        <filter id="loomarr-tv-static">
          <feTurbulence type="fractalNoise" baseFrequency="0.95" numOctaves={2} stitchTiles="stitch" />
          {/* Desaturate to white, then hard-contrast the alpha so the smooth noise collapses
              into sharp discrete specks — the "snow" read, not a soft cellular grid. */}
          <feColorMatrix type="saturate" values="0" />
          <feComponentTransfer>
            <feFuncA type="linear" slope="1.4" intercept="-0.2" />
          </feComponentTransfer>
        </filter>
        <rect width="100%" height="100%" filter="url(#loomarr-tv-static)" />
      </svg>
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
