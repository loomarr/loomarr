import { isWeb, useTheme, View } from "@tamagui/core";
import type { ComponentProps, ReactNode } from "react";

import type { ItemLayoutProps } from "../../item-layout";
import { lineHeightPx, withAlpha } from "../../text";
import { TamaguiText } from "../../text-base";
import { brandChroma, type Density, typography } from "../../tokens";

type BadgeTone =
  | "caution"
  | "danger"
  | "info"
  | "live"
  | "lock"
  | "neutral"
  | "onair"
  | "override"
  | "signal"
  | "success"
  | "suggest"
  | "tune"
  | "warning";
type BadgeShape = "pill" | "square";
type BadgeProps = Omit<ComponentProps<typeof View>, "alignSelf" | "children"> &
  ItemLayoutProps & {
    children: ReactNode;
    density?: Density;
    /** Square only: a glyph before the label, 4px from it. On web it inherits the badge's colour. */
    icon?: ReactNode;
    /**
     * `pill` is the rounded state chip (danger/info/live/neutral/success/warning). `square` is the
     * machine-data tag (#970 legacy Badge): mono, sentence case (#2778), a hairline-free tinted-15% background,
     * no border. The two shapes intentionally do not share a tone vocabulary below.
     */
    shape?: BadgeShape;
    /** Square only: the accent tones' tint strength, lowered for a badge on a raised surface. */
    tintOpacity?: number;
    /** Web only: the browser's tooltip, for a tag whose short label needs explaining. */
    title?: string;
    tone?: BadgeTone;
  };

const badgeTone = {
  danger: { backgroundColor: "stateDangerSurface", borderColor: "stateDanger", color: "stateDanger" },
  info: { backgroundColor: "stateInfoSurface", borderColor: "stateInfo", color: "stateInfo" },
  live: { backgroundColor: "stateLiveSurface", borderColor: "stateLive", color: "stateLive" },
  neutral: {
    backgroundColor: "surfaceElevated",
    borderColor: "actionDisabled",
    color: "contentSecondary",
  },
  // A channel override or Custom value (#1909 Q5): neutral with a foreground label.
  override: {
    backgroundColor: "surfaceElevated",
    borderColor: "borderControl",
    color: "contentPrimary",
  },
  success: {
    backgroundColor: "stateSuccessSurface",
    borderColor: "stateSuccess",
    color: "stateSuccess",
  },
  warning: {
    backgroundColor: "stateWarningSurface",
    borderColor: "stateWarning",
    color: "stateWarning",
  },
} as const;

// "#0B0C0E" + 0.15 → "rgba(11,12,14,0.15)", the same formula CSS `color-mix(in srgb, X 15%,
// transparent)` resolves to (shared with Text's halo: `withAlpha` lives in ../text).
const squareBadgeTone = (theme: ReturnType<typeof useTheme>, tint: number) =>
  ({
    // The only square tone with a flat, non-tinted background — the legacy ledger's "neutral".
    neutral: { backgroundColor: theme.surfaceElevated.val, color: theme.contentSecondary.val },
    // A channel override or Custom value (#1909 Q5): neutral, with a foreground label and a control
    // hairline. The word carries the meaning; amber stays for focus and the primary action.
    override: {
      backgroundColor: "surfaceElevated",
      borderColor: "borderControl",
      color: theme.contentPrimary.get(),
    },
    tune: { backgroundColor: withAlpha(theme.stateInfo.val, tint), color: theme.stateInfo.val },
    lock: { backgroundColor: withAlpha(theme.stateSuccess.val, tint), color: theme.stateSuccess.val },
    caution: { backgroundColor: withAlpha(theme.stateWarning.val, tint), color: theme.stateWarning.val },
    // `danger`/`stateDanger` already resolves to the AA-safe onair-300 stop; the tint itself uses
    // the stronger base onair red, same split as the legacy `bg-onair-tint-15 text-onair-300`.
    onair: { backgroundColor: withAlpha(theme.guideOnAir.val, tint), color: theme.stateDanger.val },
    // The brand chroma stops are theme-invariant (same as `guide.onAir`), so the base suggest tint
    // comes straight from `brandChroma` rather than a theme entry.
    suggest: { backgroundColor: withAlpha(brandChroma[4], tint), color: theme.accentSuggest.val },
    signal: { backgroundColor: withAlpha(theme.actionPrimary.val, tint), color: theme.actionPrimary.val },
  }) as const;

const Badge = ({
  alignSelf,
  children,
  density = "pointer",
  flexShrink,
  icon,
  shape = "pill",
  tintOpacity = 0.15,
  tone = "neutral",
  ...props
}: BadgeProps) => {
  const theme = useTheme();

  if (shape === "square") {
    const squareTones = squareBadgeTone(theme, tintOpacity);
    const colors: { backgroundColor: string; borderColor?: string; color: string } =
      tone in squareTones ? squareTones[tone as keyof typeof squareTones] : squareTones.neutral;
    // A hairline takes its 1px from the padding, so an override badge is no taller than Default.
    const hairline = colors.borderColor ? 1 : 0;
    // On web the tag behaves as an inline element: it hugs its label in a block parent, follows its
    // row's alignment, and may shrink. The colour sits on the wrapper so an icon's `currentColor`
    // and a hover class on the badge both reach the label.
    const placement = isWeb
      ? ({
          display: "inline-flex",
          render: "span",
          style: { color: colors.color },
          width: "fit-content",
        } as const)
      : ({ alignSelf: "flex-start" } as const);
    return (
      <View
        {...props}
        {...placement}
        {...(alignSelf !== undefined && { alignSelf })}
        alignItems="center"
        backgroundColor={colors.backgroundColor}
        borderColor={colors.borderColor}
        borderRadius={4}
        borderWidth={hairline}
        flexDirection="row"
        flexShrink={flexShrink ?? (isWeb ? 1 : 0)}
        gap={icon ? 4 : undefined}
        paddingHorizontal={6 - hairline}
        paddingVertical={2 - hairline}
      >
        {icon}
        <TamaguiText
          color={isWeb ? undefined : colors.color}
          fontFamily="data"
          fontSize={11}
          fontWeight="500"
          lineHeight={lineHeightPx(14)}
          whiteSpace={isWeb ? "inherit" : undefined}
        >
          {children}
        </TamaguiText>
      </View>
    );
  }

  const colors = badgeTone[tone as keyof typeof badgeTone] ?? badgeTone.neutral;
  return (
    <View
      {...props}
      alignItems="center"
      alignSelf={alignSelf ?? "flex-start"}
      backgroundColor={colors.backgroundColor}
      borderColor={colors.borderColor}
      borderRadius="round"
      borderWidth={1}
      flexShrink={0}
      paddingHorizontal="inline"
      paddingVertical={4}
    >
      <TamaguiText
        color={colors.color}
        fontFamily="data"
        fontSize={typography[density].metadata.size}
        fontWeight="600"
        lineHeight={lineHeightPx(typography[density].metadata.lineHeight)}
      >
        {children}
      </TamaguiText>
    </View>
  );
};

export type { BadgeShape, BadgeTone };
export { Badge };
