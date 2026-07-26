import { cn } from "@/lib";
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

// The palette monograms cycle through. Drawn from the design tokens rather than invented, so
// the rail stays inside the product's colour language however many channels exist.
const IDENT_COLORS = [
  "var(--color-tune)",
  "var(--color-suggest)",
  "var(--color-signal)",
  "var(--color-onair)",
  "var(--color-tune)",
] as const;

// monogramOf takes the initials of the first two significant words — "1980s Action Heroes" →
// "AH", "Springfield Classics" → "SC". Leading numbers are skipped: a channel named "90s
// Action" should read "9A" at best, and "AC" is more recognisable than a digit pair.
const monogramOf = (name: string): string => {
  const words = name
    .split(/[\s—–-]+/)
    .map((w) => w.replace(/[^\p{L}\p{N}]/gu, ""))
    .filter(Boolean);
  const alpha = words.filter((w) => /^\p{L}/u.test(w));
  const source = alpha.length > 0 ? alpha : words;
  const [first, second] = source;
  // "CH" rather than empty: a channel named only with punctuation still needs a mark, and a
  // blank square reads as a failed image load.
  if (!first) return "CH";
  if (!second) return first.slice(0, 2).toUpperCase();
  return ((first[0] ?? "") + (second[0] ?? "")).toUpperCase();
};

// Colour is keyed on the channel NUMBER, so a channel keeps its colour as long as its number
// is stable — renaming it does not change how the rail looks.
const colorFor = (num: number) => IDENT_COLORS[Math.abs(num) % IDENT_COLORS.length] ?? IDENT_COLORS[0];

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
