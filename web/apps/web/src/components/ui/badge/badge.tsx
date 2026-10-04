import { brandChroma, semanticColors } from "@loomarr/design-system";
import type { BadgeProps, BadgeVariant } from "./badge.type";

// Badge — the tinted chip, and THE component the §2.1 badge rule is about, backed by
// @loomarr/design-system's shared token source (#970 PR B). design-system's own `Badge` primitive
// grew this component's exact tone set and "square" shape in the same change, but it renders
// through Tamagui, which has no exact className-override guarantee (model-discover.tsx relies on
// `className="shrink-0"` winning via tailwind-merge) and no native `title` passthrough contract —
// so this file keeps the legacy <span> and sources only its colours from the shared tokens,
// dropping the `class-variance-authority` dependency this file used to carry.
//
// Badges carry machine data, so mono + uppercase (§2.2). Accent variants pair the
// 15% composited tint with the AA-safe text stop — onair/suggest use their -300
// stops (§2.1 badge rule): the base stops fail AA on the composited tint (4.02:1 and 3.86:1).
// `semanticColors.state.danger`/`accent.suggest` already resolve to those -300 stops.
const withAlpha = (hex: string, alpha: number) => {
  const n = Number.parseInt(hex.slice(1, 7), 16);
  return `rgba(${(n >> 16) & 255},${(n >> 8) & 255},${n & 255},${alpha})`;
};

const ACCENT_COLOR: Record<Exclude<BadgeVariant, "neutral">, { tint: string; text: string }> = {
  tune: { tint: semanticColors.state.info, text: semanticColors.state.info },
  lock: { tint: semanticColors.state.success, text: semanticColors.state.success },
  caution: { tint: semanticColors.state.warning, text: semanticColors.state.warning },
  onair: { tint: semanticColors.guide.onAir, text: semanticColors.state.danger },
  suggest: { tint: brandChroma[4], text: semanticColors.accent.suggest },
  signal: { tint: semanticColors.action.primary, text: semanticColors.action.primary },
};

const Badge = ({ className, variant = "neutral", style, tintOpacity = 0.15, ...props }: BadgeProps) => {
  const background =
    variant === "neutral"
      ? semanticColors.surface.elevated
      : withAlpha(ACCENT_COLOR[variant].tint, tintOpacity);
  const text = variant === "neutral" ? semanticColors.content.secondary : ACCENT_COLOR[variant].text;
  return (
    <span
      className={className}
      style={{
        alignItems: "center",
        backgroundColor: background,
        borderRadius: 4,
        color: text,
        display: "inline-flex",
        fontFamily: "var(--font-mono)",
        fontSize: 11,
        fontWeight: 500,
        letterSpacing: "0.025em",
        paddingBlock: 2,
        paddingInline: 6,
        textTransform: "uppercase",
        width: "fit-content",
        ...style,
      }}
      {...props}
    />
  );
};

export { Badge };
