import { isWeb, useTheme, View } from "@tamagui/core";
import type { ComponentProps, ElementType } from "react";
import { createElement } from "react";

import { TamaguiText } from "../text-base";
import { type Density, type TextRole, typography } from "../tokens";

// Text is its own module, apart from the other primitives: it is on nearly every surface, eager
// code included, and importing it must not also load what the other primitives need (a Tooltip
// and its floating-ui stack cost the web's initial bundle ~100 KiB).

type TamaguiTextProps = ComponentProps<typeof TamaguiText>;
type TextTone =
  | "danger"
  | "info"
  | "inverse"
  | "live"
  | "muted"
  | "onDecade"
  | "primary"
  | "secondary"
  | "signal"
  | "success"
  | "identOnair"
  | "identSignal"
  | "identSuggest"
  | "identTune"
  | "suggest"
  | "warning";
type TextProps = Omit<
  TamaguiTextProps,
  "color" | "fontFamily" | "fontSize" | "fontWeight" | "letterSpacing" | "lineHeight" | "role"
> & {
  /**
   * Renders as this host element instead of Tamagui's own Text (web only; ignored on native,
   * where Tamagui's Text is already the host). For a caller that needs `<p>`, `<dt>`, or another
   * element Tamagui's Text does not expose a tag for (#970 PR B checkpoint 2's Caption).
   *
   * Bypasses `halo`: a polymorphic readout has never needed the snow/picture treatment, and the
   * halo's two-layer markup assumes Tamagui's own Text underneath.
   */
  as?: ElementType;
  density?: Density;
  /**
   * A readout over snow or a picture (#1627 B4): the canvas as a two-layer halo, tight under the
   * glyphs and wide around them, so the text holds on any ground. React Native takes one text
   * shadow, so the wide layer is a second, hidden copy of the text behind the first.
   */
  halo?: boolean;
  /** Uppercase with wide tracking — the section-label voice ("POD · 1:10", a shouted Caption). */
  shout?: boolean;
  /** The role's size at weight 600: a checked or selected choice's label (neutral-pressed, #1909). */
  strong?: boolean;
  textRole: TextRole;
  tone?: TextTone;
  /** Letter spacing in em, for tracked-out mono readouts ("CH 7" at 0.16, "TUNING IN" at 0.24). Overrides the role's own. */
  tracking?: number;
};

/** The concrete value of a theme colour key, for hosts that take inline style rather than a token. */
const themeVal = (theme: ReturnType<typeof useTheme>, key: string) =>
  (theme as unknown as Record<string, { val: string }>)[key]?.val;

/**
 * A line height in px for a Tamagui text prop. Tamagui 3 reads a bare numeric `lineHeight` as a
 * ratio of the font size on web and native alike, so an absolute height must be an explicit px string.
 */
const lineHeightPx = (px: number) => `${px}px`;

const SHOUT_TRACKING = 0.025;

// "#0B0C0E" + 0.9 → "rgba(11,12,14,0.9)": the halo is the theme's canvas, part-transparent.
const withAlpha = (hex: string, alpha: number) => {
  const n = Number.parseInt(hex.slice(1, 7), 16);
  return `rgba(${(n >> 16) & 255},${(n >> 8) & 255},${n & 255},${alpha})`;
};

const dataRoles = new Set<TextRole>([
  "metadata",
  "time",
  "data",
  "channelNumber",
  "bandNumber",
  "code",
  "cardChannelNumber",
  "cardTime",
  "mono",
  "guideNumber",
]);
const mutedRoles = new Set<TextRole>([
  "label",
  "section",
  "metadata",
  "time",
  "cardMeta",
  "cardTime",
  "mono",
]);
// No role defaults to amber: amber means focus or the primary action (#1909), so a number is
// foreground at rest, and a display numeral (direct-tune digits, a pairing code) opts in with
// `tone="signal"`.
const textTones = {
  danger: "stateDanger",
  info: "stateInfo",
  inverse: "contentInverse",
  live: "stateLive",
  muted: "contentMuted",
  // A channel number on its decade's fill (#2767).
  onDecade: "decadeInk",
  primary: "contentPrimary",
  secondary: "contentSecondary",
  signal: "actionPrimary",
  success: "stateSuccess",
  suggest: "accentSuggest",
  // The monogram ident's letters on their own wash (#3011).
  identOnair: "identLetterOnair",
  identSignal: "identLetterSignal",
  identSuggest: "identLetterSuggest",
  identTune: "identLetterTune",
  warning: "stateWarning",
} as const;

const roleTracking = (textRole: TextRole, size: number) =>
  textRole === "display"
    ? -0.8
    : textRole === "title"
      ? -0.25
      : textRole === "screenTitle"
        ? size * -0.02
        : textRole === "section"
          ? 2
          : 0;

// Tamagui's web Text hands `accessibilityLiveRegion` to the DOM verbatim, where React rejects it and
// no screen reader hears it; the web spelling is `aria-live` (RN's "none" is the web's "off").
const webLiveRegion = (
  props: Omit<TextProps, "as" | "density" | "halo" | "shout" | "strong" | "textRole" | "tone" | "tracking">,
) => {
  if (!isWeb || props.accessibilityLiveRegion === undefined) return props;
  const { accessibilityLiveRegion, ...rest } = props;
  return {
    ...rest,
    "aria-live": accessibilityLiveRegion === "none" ? "off" : accessibilityLiveRegion,
  } as typeof rest;
};

const Text = ({
  as,
  density = "pointer",
  halo,
  shout,
  strong,
  textRole,
  tone,
  tracking,
  ...given
}: TextProps) => {
  const props = webLiveRegion(given);
  const theme = useTheme();
  const role = typography[density][textRole];
  const value = strong ? { ...role, weight: "600" as const } : role;
  const colorToken = tone
    ? textTones[tone]
    : mutedRoles.has(textRole)
      ? "contentSecondary"
      : "contentPrimary";
  const resolvedTracking =
    tracking === undefined
      ? shout
        ? value.size * SHOUT_TRACKING
        : roleTracking(textRole, value.size)
      : value.size * tracking;
  const face = {
    color: colorToken,
    fontFamily: dataRoles.has(textRole) ? "data" : "body",
    fontSize: value.size,
    fontWeight: value.weight,
    letterSpacing: resolvedTracking,
    lineHeight: lineHeightPx(value.lineHeight),
    // Only when shouting: `face` spreads after the caller's props, so an always-present key
    // would erase a caller's own `textTransform="uppercase"` (the Guide grid's labels).
    ...(shout && { textTransform: "uppercase" as const }),
  } as const;

  if (isWeb && as) {
    const Tag = as;
    return createElement(Tag, {
      ...props,
      style: {
        color: themeVal(theme, colorToken),
        fontFamily: dataRoles.has(textRole) ? typography.family.data.web : typography.family.body.web,
        fontSize: value.size,
        fontWeight: value.weight,
        letterSpacing: resolvedTracking,
        lineHeight: `${value.lineHeight}px`,
        textTransform: face.textTransform,
        ...(props as { style?: object }).style,
      },
    });
  }

  if (!halo) return <TamaguiText {...props} {...face} />;

  const canvas = theme.surfaceCanvas.val;
  return (
    <View position="relative">
      {/* The wide layer: the same glyphs, hidden, glowing canvas around the text. */}
      <TamaguiText
        {...face}
        aria-hidden
        numberOfLines={props.numberOfLines}
        position="absolute"
        top={0}
        left={0}
        right={0}
        textShadowColor={withAlpha(canvas, 0.85)}
        textShadowOffset={{ height: 0, width: 0 }}
        textShadowRadius={18}
      >
        {props.children}
      </TamaguiText>
      <TamaguiText
        {...props}
        {...face}
        textShadowColor={withAlpha(canvas, 0.9)}
        textShadowOffset={{ height: 1, width: 0 }}
        textShadowRadius={2}
      />
    </View>
  );
};

export type { TextProps, TextTone };
export { lineHeightPx, Text, textTones, withAlpha };
