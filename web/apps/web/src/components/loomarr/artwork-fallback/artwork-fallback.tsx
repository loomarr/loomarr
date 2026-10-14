import { channelIdentHue, monogramOf } from "@loomarr/core/guide";
import { cn } from "@/lib/utils";
import type { ArtworkFallbackProps } from "./artwork-fallback.type";

// ArtworkFallback — missing or failed artwork (#1822 evidence: "a restrained line motif with a
// channel monogram where available"). Keeps the caller's own geometry (16:9 channel art, 2:3
// posters); this only fills the box with a neutral surface, a few faint diagonal lines and, when
// a channel is known, its monogram — never a broken-image glyph or invented photography.
//
// CSS gradients, not an inline SVG string: there is no per-channel paste-able content here (the
// monogram and hue are both derived, §22's raster-only rule is about operator-pasted logos), but
// a plain `background-image` costs nothing and keeps this component inert markup.
const LINES =
  "repeating-linear-gradient(135deg, transparent 0 24px, color-mix(in srgb, var(--color-static-400) 16%, transparent) 24px 25px)";

const ArtworkFallback = ({ channel, className }: ArtworkFallbackProps) => {
  const hue = channel ? `var(--color-${channelIdentHue(channel.number)})` : undefined;
  return (
    <div
      role="presentation"
      style={{ backgroundImage: LINES }}
      className={cn("flex size-full items-center justify-center bg-static-800", className)}
    >
      {channel && (
        <span
          style={{ color: hue }}
          className="font-mono font-semibold text-base tracking-[0.08em] opacity-75"
        >
          {monogramOf(channel.name)}
        </span>
      )}
    </div>
  );
};

export { ArtworkFallback };
