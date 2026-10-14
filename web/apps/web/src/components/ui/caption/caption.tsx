import { semanticColors } from "@loomarr/design-system";
import { cn } from "@/lib/utils";
import type { CaptionProps } from "./caption.type";

// Caption — mono metadata that rides alongside content (§2.2, §5.1c), sourcing its colour, font
// and uppercase treatment from @loomarr/design-system's shared tokens (#970 PR B checkpoint 2).
// design-system's `Text` primitive grew a web polymorphic `as` and a `shout` variant in the same
// change, built for exactly this component — but this file keeps its own <Tag> rather than
// delegating to `Text`: `shout`'s tracking-wide is overridden at 3 real call sites via
// `className="tracking-[0.06em]"` (guide-page.tsx, channel-lineup-editor.tsx), which only works
// because `tracking-wide` stays a literal Tailwind class that `cn`'s tailwind-merge can drop in
// favour of the caller's class. `Text`'s `as` path sets letterSpacing as an inline style, which
// always wins over a class — delegating would silently freeze those 3 captions at the shout
// default instead of their intended wider tracking. `uppercase` (never overridden anywhere) and
// the colour/font moved to inline style below; `tracking-wide` stays a class for exactly that
// reason.
//
// EXTRACTED FROM 21 HAND-ROLLED COPIES across 11 components. They had drifted to three
// different sizes (10px, 10.5px, 11px) — all off-scale, because the type scale bottomed out at
// 12px and each component invented its own smaller value. `text-2xs` (11px) is now the
// sanctioned caption step and this is the only component that should use it.
//
// "If it came from a machine, it's mono" — a duration, a clock time, a channel number, an id,
// an era. Prose that a person wrote is not a caption, however small it renders.
const Caption = ({
  tone = "muted",
  shout = false,
  as: Tag = "span",
  className,
  style,
  ...rest
}: CaptionProps) => (
  <Tag
    className={cn(shout && "tracking-wide", className)}
    style={{
      color: tone === "muted" ? semanticColors.content.secondary : semanticColors.content.primary,
      fontFamily: "var(--font-mono)",
      // No explicit line-height: the legacy `text-2xs` utility set none either, so this keeps
      // the browser's own normal metric rather than inventing a value that wasn't there before.
      fontSize: 11,
      textTransform: shout ? "uppercase" : undefined,
      ...style,
    }}
    {...rest}
  />
);

export { Caption };
