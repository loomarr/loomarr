import { cn } from "@/lib";
import type { CaptionProps } from "./caption.type";

// Caption — mono metadata that rides alongside content (§2.2, §5.1c).
//
// EXTRACTED FROM 21 HAND-ROLLED COPIES across 11 components. They had drifted to three
// different sizes (10px, 10.5px, 11px) — all off-scale, because the type scale bottomed out at
// 12px and each component invented its own smaller value. `text-2xs` (11px) is now the
// sanctioned caption step and this is the only component that should use it.
//
// "If it came from a machine, it's mono" — a duration, a clock time, a channel number, an id,
// an era. Prose that a person wrote is not a caption, however small it renders.
const Caption = ({ tone = "muted", shout = false, as: Tag = "span", className, ...rest }: CaptionProps) => (
  <Tag
    className={cn(
      "font-mono text-2xs",
      tone === "muted" ? "text-static-400" : "text-static-100",
      shout && "uppercase tracking-wide",
      className,
    )}
    {...rest}
  />
);

export { Caption };
