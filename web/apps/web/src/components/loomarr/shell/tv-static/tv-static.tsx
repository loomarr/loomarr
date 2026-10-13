import { cn } from "@/lib/utils";
import { type SnowTile, useSnowTile } from "./snow-tile";
import type { TvStaticProps } from "./tv-static.type";

// TvStatic — the CRT snow behind idle surfaces (login, wizard, empty states) and NOWHERE
// near data, tables, or forms (§1). A full-bleed noise layer that FLICKERS like real TV
// static: the noise layer is oversized and its transform jumps between offsets on a stepped
// keyframe (`animate-tv-snow`, defined in the token theme), so the grain shifts a few times a
// second instead of sitting dead — a still render just reads as flat texture.
//
// The noise itself is one pre-rendered tile, repeated (see snow-tile.ts for the snow and why).
// Behind idle surfaces it arrives AFTER the first frame and fades in, so the content it sits behind
// paints first; the wash asks for it at once.
//
// Two strengths of the one snow:
// - `idle` (default): a faint veil (~11%) with a whisper of scanlines. Gated behind
//   `motion-safe:` — the whole layer is `hidden` and only reappears when motion is allowed (and
//   the tile is never rendered without it). That is ALSO the signal the visual suite pins
//   (`reducedMotion: "reduce"`, playwright.shared), so idle baselines never rasterize noise (§5.2, §17).
// - `wash`: the tuner's channel-switch snow. It is what the viewer lands on INSTEAD of a picture,
//   so it is present under reduced motion (only the flicker stops). The snow is opaque, under a
//   static-950 layer at 90% and firmer scanlines, so the readout over it stays legible.
// Inert regardless: aria-hidden + pointer-events-none.
//
// `data-snow-pending` marks a layer whose tile has not arrived yet; the visual suite waits for it
// to clear, as it waits for fonts.

// Oversized so the animated transform never exposes an edge. Until the tile arrives the layer is
// empty and fully transparent; then it fades up to its class opacity. The fade is a transition, so
// it rides alongside the flicker's `animation`, and the reduced-motion killswitch makes it instant.
const AnalogSnow = ({ tile, className }: { tile: SnowTile | undefined; className: string }) => (
  <div
    className={cn("absolute inset-[-50%] size-[200%]", className)}
    style={tile ? { ...tile, transition: "opacity var(--duration-base) ease-out" } : { opacity: 0 }}
  />
);

const reducedMotion = () => globalThis.matchMedia?.("(prefers-reduced-motion: reduce)").matches === true;

const TvStatic = ({ variant = "idle", className }: TvStaticProps) => {
  const wash = variant === "wash";
  const tile = useSnowTile(wash || !reducedMotion(), !wash);
  const pending = tile ? undefined : "";
  return wash ? (
    <div
      data-snow="wash"
      data-snow-pending={pending}
      className={cn("pointer-events-none absolute inset-0 overflow-hidden", className)}
      aria-hidden
    >
      <AnalogSnow tile={tile} className="motion-safe:animate-tv-snow" />
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
      data-snow-pending={pending}
      className={cn(
        "pointer-events-none absolute inset-0 hidden overflow-hidden motion-safe:block",
        className,
      )}
      aria-hidden
    >
      {/* Opacity lives on the snow so the flicker rides a steady, subtle wash (~11%). */}
      <AnalogSnow tile={tile} className="opacity-[0.11] motion-safe:animate-tv-snow" />
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
};

export { TvStatic };
