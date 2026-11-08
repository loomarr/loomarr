import { channelIdentHue, monogramOf } from "@loomarr/core/guide";
import { cn } from "@/lib/utils";
import type { ArtworkFallbackProps } from "./artwork-fallback.type";

// ArtworkFallback — generated art for a slot with no picture (#2762): a soft wash in the channel's
// own identity hue with its monogram set large, so a card with no still still looks finished. Keeps
// the caller's own geometry (16:9 channel art, 2:3 posters). No hatching and never a blank box, and
// never AI art: it is only the channel's hue and letters. A title with no channel sets its own
// letters, in a hue picked from `seed`.
const ArtworkFallback = ({ channel, label, seed = 0, className }: ArtworkFallbackProps) => {
  const hue = `var(--color-${channelIdentHue(channel?.number ?? seed)})`;
  const words = channel?.name ?? label;
  return (
    <div
      role="presentation"
      style={{
        backgroundImage: `linear-gradient(155deg, color-mix(in srgb, ${hue} 34%, var(--color-static-900)) 0%, var(--color-static-900) 88%)`,
      }}
      className={cn("bg-static-900 flex size-full items-center justify-center", className)}
    >
      {words && (
        <span
          style={{ color: hue }}
          className="font-mono text-3xl font-semibold tracking-[0.08em] opacity-80"
        >
          {monogramOf(words)}
        </span>
      )}
    </div>
  );
};

export { ArtworkFallback };
