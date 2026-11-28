import { isWeb, useTheme, View } from "@tamagui/core";
import type { ComponentProps, CSSProperties, ElementType } from "react";
import { createElement } from "react";

import type { ItemLayoutProps } from "../item-layout";
import { TamaguiText } from "../text-base";
import {
  type DecadeTint,
  type Density,
  type LeadingStep,
  leadingSteps,
  roleLetterSpacing,
  type TextRole,
  typography,
} from "../tokens";

// Text is its own module, apart from the other primitives: it is on nearly every surface, eager
// code included, and importing it must not also load what the other primitives need (a Tooltip
// and its floating-ui stack cost the web's initial bundle ~100 KiB).

type TamaguiTextProps = ComponentProps<typeof TamaguiText>;
type TextTone =
  | DecadeTint
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
  | "alignSelf"
  | "color"
  | "flex"
  | "flexShrink"
  | "fontFamily"
  | "fontSize"
  | "fontWeight"
  | "letterSpacing"
  | "lineHeight"
  | "minWidth"
  | "role"
> &
  ItemLayoutProps & {
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
     * The flex-item props: how the text sizes inside the row or column around it. With `alignSelf`
     * they are the only layout a Text takes; space around it is the parent Stack's gap or padding.
     * They hold on every host, `as` and `halo` included.
     *
     * `flex` grows the text into the free space, as Tamagui's `flex` does: grow by n, shrink 1, from a
     * zero basis (Tailwind's `flex-1` is `flex={1}`).
     */
    flex?: number;
    /** Whether the text gives way when its row is short of room: 1 (`shrink`) or 0 (`shrink-0`). */
    flexShrink?: number;
    /**
     * The narrowest the text may get, in px. `0` (`min-w-0`) lets a text in a row shrink below its
     * longest word, so `numberOfLines` can truncate it.
     */
    minWidth?: number;
    /**
     * A readout over snow or a picture (#1627 B4): the canvas as a two-layer halo, tight under the
     * glyphs and wide around them, so the text holds on any ground. React Native takes one text
     * shadow, so the wide layer is a second, hidden copy of the text behind the first.
     */
    halo?: boolean;
    /** Line height as a multiple of the size (`none` is 1), in place of the role's own height in px. */
    leading?: LeadingStep;
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

type HostTextStyleProps = Pick<
  TamaguiTextProps,
  | "cursor"
  | "fontVariant"
  | "numberOfLines"
  | "overflowWrap"
  | "textAlign"
  | "textDecorationLine"
  | "wordBreak"
>;

/**
 * The Tamagui text-style props a caller of the `as` host may set, as the inline style they mean.
 * That branch hands a plain DOM element its props, where Tamagui's own Text would have turned each
 * of these into CSS: left in the props they reach the DOM as attributes React warns about and no
 * browser reads. `numberOfLines` is `truncate` for 1 (one line, an ellipsis) and a line clamp for
 * more, as Tamagui draws it.
 */
const hostTextStyle = ({
  cursor,
  fontVariant,
  numberOfLines,
  overflowWrap,
  textAlign,
  textDecorationLine,
  wordBreak,
}: HostTextStyleProps): CSSProperties => {
  // Tamagui types each prop as a token, media object or CSS keyword too; a host takes the keyword.
  const lines = typeof numberOfLines === "number" ? numberOfLines : 0;
  return {
    ...(cursor && { cursor: cursor as CSSProperties["cursor"] }),
    ...(textAlign && { textAlign: textAlign as CSSProperties["textAlign"] }),
    ...(textDecorationLine && {
      textDecorationLine: textDecorationLine as CSSProperties["textDecorationLine"],
    }),
    ...(Array.isArray(fontVariant) && fontVariant.length > 0 && { fontVariant: fontVariant.join(" ") }),
    ...(overflowWrap && { overflowWrap: overflowWrap as CSSProperties["overflowWrap"] }),
    ...(wordBreak && { wordBreak: wordBreak as CSSProperties["wordBreak"] }),
    ...(lines === 1 && { overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }),
    ...(lines > 1 && {
      WebkitBoxOrient: "vertical",
      WebkitLineClamp: lines,
      display: "-webkit-box",
      overflow: "hidden",
    }),
  };
};

type FlexItem = Pick<TextProps, "alignSelf" | "flex" | "flexShrink" | "minWidth">;

/** The flex-item props a caller set, and only those, so an unset one never overrides a default. */
const flexItem = ({ alignSelf, flex, flexShrink, minWidth }: FlexItem) => ({
  ...(alignSelf !== undefined && { alignSelf }),
  ...(flex !== undefined && { flex }),
  ...(flexShrink !== undefined && { flexShrink }),
  ...(minWidth !== undefined && { minWidth }),
});

/** The flex-item props as an `as` host's inline style, with `flex` expanded as Tamagui's web Text does. */
const hostFlexStyle = ({ alignSelf, flex, flexShrink, minWidth }: FlexItem): CSSProperties => ({
  ...(alignSelf !== undefined && { alignSelf }),
  ...(flex !== undefined && { flexBasis: 0, flexGrow: flex, flexShrink: 1 }),
  ...(flexShrink !== undefined && { flexShrink }),
  ...(minWidth !== undefined && { minWidth }),
});

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
  "monogram",
  "numeral",
  "pairCode",
  "microBadge",
  "footnote",
  "stationTag",
]);
const mutedRoles = new Set<TextRole>([
  "label",
  "section",
  "metadata",
  "time",
  "cardMeta",
  "cardTime",
  "mono",
  "footnote",
]);
// No role defaults to amber: amber means focus or the primary action (#1909), so a number is
// foreground at rest, and a display numeral (direct-tune digits, a pairing code) opts in with
// `tone="signal"`.
const textTones = {
  // The Cross's decade tints (§2.1), named as `decadeTint` returns them: the watched channel's
  // numeral and the By decade headings. Each follows the theme.
  decadeTint1950: "decadeTint1950",
  decadeTint1960: "decadeTint1960",
  decadeTint1970: "decadeTint1970",
  decadeTint1980: "decadeTint1980",
  decadeTint1990: "decadeTint1990",
  decadeTint2000: "decadeTint2000",
  decadeTintNeutral: "decadeTintNeutral",
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
  textRole in roleLetterSpacing
    ? size * roleLetterSpacing[textRole as keyof typeof roleLetterSpacing]
    : textRole === "display"
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
  props: Omit<
    TextProps,
    "as" | "density" | "halo" | "leading" | "shout" | "strong" | "textRole" | "tone" | "tracking"
  >,
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
  leading,
  shout,
  alignSelf,
  flex,
  flexShrink,
  minWidth,
  strong,
  textRole,
  tone,
  tracking,
  ...given
}: TextProps) => {
  const props = webLiveRegion(given);
  const item = flexItem({ alignSelf, flex, flexShrink, minWidth });
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
  // A fluid role (web only) is `value.size` at wide windows and scales down with the window to
  // `fluid.min`, its letter spacing in em and its line height a multiple of the size, so both follow.
  const fluid = isWeb && "fluid" in value ? value.fluid : undefined;
  const trackingEm =
    tracking ?? (shout ? SHOUT_TRACKING : ((roleLetterSpacing as Record<string, number>)[textRole] ?? 0));
  const fontSize = fluid ? `clamp(${fluid.min}px, ${fluid.vw}vw, ${value.size}px)` : value.size;
  const letterSpacing = fluid ? `${trackingEm}em` : resolvedTracking;
  const lineHeightRatio = leading
    ? leadingSteps[leading]
    : fluid
      ? Math.round((value.lineHeight / value.size) * 1000) / 1000
      : undefined;
  const face = {
    color: colorToken,
    fontFamily: dataRoles.has(textRole) ? "data" : "body",
    fontSize: fontSize as number,
    fontWeight: value.weight,
    letterSpacing: letterSpacing as number,
    lineHeight: lineHeightRatio ?? lineHeightPx(value.lineHeight),
    // Only when shouting: `face` spreads after the caller's props, so an always-present key
    // would erase a caller's own `textTransform="uppercase"` (the Guide grid's labels).
    ...(shout && { textTransform: "uppercase" as const }),
  } as const;

  if (isWeb && as) {
    const Tag = as;
    const {
      cursor,
      fontVariant,
      numberOfLines,
      overflowWrap,
      textAlign,
      textDecorationLine,
      wordBreak,
      ...host
    } = props;
    return createElement(Tag, {
      ...host,
      style: {
        ...hostTextStyle({
          cursor,
          fontVariant,
          numberOfLines,
          overflowWrap,
          textAlign,
          textDecorationLine,
          wordBreak,
        }),
        ...hostFlexStyle(item),
        color: themeVal(theme, colorToken),
        fontFamily: dataRoles.has(textRole) ? typography.family.data.web : typography.family.body.web,
        fontSize,
        fontWeight: value.weight,
        letterSpacing,
        lineHeight: lineHeightRatio ?? `${value.lineHeight}px`,
        textTransform: face.textTransform,
        ...(props as { style?: object }).style,
      },
    });
  }

  if (!halo) return <TamaguiText {...props} {...item} {...face} />;

  const canvas = theme.surfaceCanvas.val;
  // The wrapper is the flex item here, so the flex-item props size it; the two copies fill it.
  return (
    <View position="relative" {...item}>
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
