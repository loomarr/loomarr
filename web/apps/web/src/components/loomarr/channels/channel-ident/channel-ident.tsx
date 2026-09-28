import { channelIdentHue, monogramOf } from "@loomarr/core/guide";
import { cn } from "@/lib/utils";
import type { ChannelIdentProps } from "./channel-ident.type";

// ChannelIdent — a channel's mark in the guide rail (§12, the v2 mock's rail icon).
//
// A logo when the channel has one, otherwise a derived monogram. Deriving rather than leaving
// a blank is the point: a rail of unlabelled rows is much harder to scan than one with even a
// crude mark, and most channels never get a logo uploaded.
//
// The mock hardcoded per-channel monograms and colours. Those are DERIVED here, because a
// fixed table cannot cover channels the operator creates — and a channel that fell off the
// table would render blank, which is exactly the case the fallback exists for.

// The hatch overlay the mock puts behind a monogram, so an icon-less row still reads as a
// deliberate mark rather than a flat coloured square.
const HATCH = "repeating-linear-gradient(135deg,transparent 0 3px,rgb(255 255 255/0.05) 3px 6px)";

// The letters and the hue come from `@loomarr/core/guide` (#1659, N8), so the shared guide grid
// and native draw the same mark. Colour is keyed on the channel NUMBER, so renaming a channel
// does not change how the rail looks.
const colorFor = (num: number) => `var(--color-${channelIdentHue(num)})`;

const ChannelIdent = ({ name, number, logo, size = 30, className }: ChannelIdentProps) => {
  if (logo) {
    return (
      <div
        role="img"
        aria-label={`${name} logo`}
        style={{ width: size, height: size, backgroundImage: `url(${JSON.stringify(logo)})` }}
        className={cn(
          "shrink-0 rounded-[5px] border border-border bg-background bg-center bg-cover",
          className,
        )}
      />
    );
  }

  const color = colorFor(number);
  return (
    <div
      aria-hidden="true"
      style={{
        width: size,
        height: size,
        // color-mix keeps the tint tied to the same variable as the text, so a palette change
        // moves both together rather than leaving a mismatched pair.
        backgroundColor: `color-mix(in srgb, ${color} 8%, transparent)`,
        borderColor: `color-mix(in srgb, ${color} 30%, transparent)`,
        backgroundImage: HATCH,
      }}
      className={cn("flex shrink-0 items-center justify-center rounded-[5px] border", className)}
    >
      <span
        style={{ color, fontSize: Math.round(size * 0.37) }}
        className="font-mono font-semibold tracking-[0.02em]"
      >
        {monogramOf(name)}
      </span>
    </div>
  );
};

export { ChannelIdent, monogramOf };
