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

const BADGE_COLOR: Record<BadgeVariant, { background: string; text: string }> = {
  neutral: { background: semanticColors.surface.elevated, text: semanticColors.content.secondary },
  tune: { background: withAlpha(semanticColors.state.info, 0.15), text: semanticColors.state.info },
  lock: { background: withAlpha(semanticColors.state.success, 0.15), text: semanticColors.state.success },
  caution: { background: withAlpha(semanticColors.state.warning, 0.15), text: semanticColors.state.warning },
  onair: { background: withAlpha(semanticColors.guide.onAir, 0.15), text: semanticColors.state.danger },
  suggest: { background: withAlpha(brandChroma[4], 0.15), text: semanticColors.accent.suggest },
  signal: { background: withAlpha(semanticColors.action.primary, 0.15), text: semanticColors.action.primary },
};

const Badge = ({ className, variant = "neutral", style, ...props }: BadgeProps) => {
  const colors = BADGE_COLOR[variant];
  return (
    <span
      className={className}
      style={{
        alignItems: "center",
        backgroundColor: colors.background,
        borderRadius: 4,
        color: colors.text,
        display: "inline-flex",
        flexShrink: 0,
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
