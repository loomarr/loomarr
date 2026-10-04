import { isWeb, styled, Text as TamaguiText, useTheme, View } from "@tamagui/core";
import type { ComponentProps, ElementType, ReactNode } from "react";
import { createElement, useEffect, useRef } from "react";
import { Animated, Easing } from "react-native";

import { useNativeAnimationDriver } from "../motion/animation-driver";
import { useReducedMotionPreference } from "../motion/use-reduced-motion";
import { brandChroma, type Density, type TextRole, typography } from "../tokens";
import { resolveViewportInsets, useViewportInsets } from "../viewport";

const ScreenFrame = styled(View, {
  name: "LoomarrScreen",
  flex: 1,
  width: "100%",
  minHeight: isWeb ? "100vh" : "100%",
  backgroundColor: "$surfaceCanvas",
});

type ScreenFrameProps = ComponentProps<typeof ScreenFrame>;
type ScreenProps = Omit<
  ScreenFrameProps,
  | "padding"
  | "paddingBlock"
  | "paddingBlockEnd"
  | "paddingBlockStart"
  | "paddingBottom"
  | "paddingEnd"
  | "paddingHorizontal"
  | "paddingInline"
  | "paddingInlineEnd"
  | "paddingInlineStart"
  | "paddingLeft"
  | "paddingRight"
  | "paddingStart"
  | "paddingTop"
  | "paddingVertical"
> & {
  density?: Density;
  /**
   * Chrome docked under the content, edge to edge (a phone's TabBar). It carries the bottom inset
   * itself, so the content stops above it with no bottom gutter.
   */
  footer?: ReactNode;
  /**
   * The content keeps to the platform's safe area but sets its own gutters, so a phone's picture
   * can run edge to edge (#1659 mock 5e).
   */
  flush?: boolean;
};

/**
 * Edge-to-edge application frame whose content always stays inside platform insets and the
 * distance-appropriate Loomarr gutter. Insets are supplied once by LoomarrProvider.
 */
const Screen = ({ density = "pointer", flush = false, footer, ...props }: ScreenProps) => {
  const platformInsets = useViewportInsets();
  const insets = flush ? platformInsets : resolveViewportInsets(density, platformInsets);
  if (footer)
    return (
      <ScreenFrame>
        <ScreenFrame
          {...props}
          flex={1}
          minHeight={0}
          paddingBottom={0}
          paddingLeft={insets.left}
          paddingRight={insets.right}
          paddingTop={insets.top}
        />
        {footer}
      </ScreenFrame>
    );
  return (
    <ScreenFrame
      {...props}
      paddingBottom={insets.bottom}
      paddingLeft={insets.left}
      paddingRight={insets.right}
      paddingTop={insets.top}
    />
  );
};

const Surface = styled(View, {
  name: "LoomarrSurface",
  borderColor: "$borderDecorative",
  borderWidth: 1,
  borderRadius: "$card",
  variants: {
    level: {
      canvas: { backgroundColor: "$surfaceCanvas" },
      raised: { backgroundColor: "$surfaceRaised" },
      elevated: { backgroundColor: "$surfaceElevated" },
      overlay: { backgroundColor: "$surfaceOverlay", borderRadius: "$overlay" },
      focus: { backgroundColor: "$surfaceFocus", borderColor: "$actionFocus" },
    },
  } as const,
  defaultVariants: {
    level: "raised",
  },
});

const FocusSurface = styled(Surface, {
  name: "LoomarrFocusSurface",
  borderWidth: 2,
  borderColor: "$transparent",
  variants: {
    focused: {
      false: {
        backgroundColor: "$surfaceRaised",
        borderColor: "$transparent",
      },
      true: {
        backgroundColor: "$surfaceFocus",
        borderColor: "$actionFocus",
      },
    },
  } as const,
  defaultVariants: {
    focused: false,
  },
});

type TamaguiTextProps = ComponentProps<typeof TamaguiText>;
type TextTone =
  | "danger"
  | "info"
  | "inverse"
  | "live"
  | "muted"
  | "primary"
  | "secondary"
  | "signal"
  | "success"
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
  textRole: TextRole;
  tone?: TextTone;
  /** Letter spacing in em, for tracked-out mono readouts ("CH 7" at 0.16, "TUNING IN" at 0.24). Overrides the role's own. */
  tracking?: number;
};

/** The `$token` string a theme-token colour value resolves to, stripped for direct theme lookup. */
const themeVal = (theme: ReturnType<typeof useTheme>, token: string) =>
  (theme as unknown as Record<string, { val: string }>)[token.slice(1)]?.val;

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
  "code",
  "cardChannelNumber",
  "cardTime",
  "cardInitials",
  "guideMeta",
  "guideLabel",
  "guideNumber",
  "guideIdent",
  "guideNow",
]);
const mutedRoles = new Set<TextRole>([
  "label",
  "section",
  "metadata",
  "time",
  "cardMeta",
  "cardTime",
  "guideMeta",
  "guideLabel",
]);
const amberRoles = new Set<TextRole>(["channelNumber", "code", "cardChannelNumber", "guideNumber"]);
const textTones = {
  danger: "$stateDanger",
  info: "$stateInfo",
  inverse: "$contentInverse",
  live: "$stateLive",
  muted: "$contentMuted",
  primary: "$contentPrimary",
  secondary: "$contentSecondary",
  signal: "$actionPrimary",
  success: "$stateSuccess",
  warning: "$stateWarning",
} as const;

const roleTracking = (textRole: TextRole, size: number) =>
  textRole === "display"
    ? -0.8
    : textRole === "title"
      ? -0.25
      : textRole === "section"
        ? 2
        : textRole === "guideLabel"
          ? size * 0.04
          : 0;

const Text = ({ as, density = "pointer", halo, shout, textRole, tone, tracking, ...props }: TextProps) => {
  const theme = useTheme();
  const value = typography[density][textRole];
  const colorToken = tone
    ? textTones[tone]
    : amberRoles.has(textRole)
      ? "$actionPrimary"
      : mutedRoles.has(textRole)
        ? "$contentSecondary"
        : "$contentPrimary";
  const resolvedTracking =
    tracking === undefined
      ? shout
        ? value.size * SHOUT_TRACKING
        : roleTracking(textRole, value.size)
      : value.size * tracking;
  const face = {
    color: colorToken,
    fontFamily: dataRoles.has(textRole) ? "$data" : "$body",
    fontSize: value.size,
    fontWeight: value.weight,
    letterSpacing: resolvedTracking,
    lineHeight: value.lineHeight,
    textTransform: shout ? ("uppercase" as const) : undefined,
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

type BadgeTone =
  | "caution"
  | "danger"
  | "info"
  | "live"
  | "lock"
  | "neutral"
  | "onair"
  | "signal"
  | "success"
  | "suggest"
  | "tune"
  | "warning";
type BadgeShape = "pill" | "square";
type BadgeProps = Omit<ComponentProps<typeof View>, "children"> & {
  children: ReactNode;
  density?: Density;
  /**
   * `pill` is the rounded state chip (danger/info/live/neutral/success/warning). `square` is the
   * machine-data tag (#970 legacy Badge): mono, uppercase, a hairline-free tinted-15% background,
   * no border. The two shapes intentionally do not share a tone vocabulary below.
   */
  shape?: BadgeShape;
  tone?: BadgeTone;
};

const badgeTone = {
  danger: { backgroundColor: "$stateDangerSurface", borderColor: "$stateDanger", color: "$stateDanger" },
  info: { backgroundColor: "$stateInfoSurface", borderColor: "$stateInfo", color: "$stateInfo" },
  live: { backgroundColor: "$stateLiveSurface", borderColor: "$stateLive", color: "$stateLive" },
  neutral: {
    backgroundColor: "$surfaceElevated",
    borderColor: "$actionDisabled",
    color: "$contentSecondary",
  },
  success: {
    backgroundColor: "$stateSuccessSurface",
    borderColor: "$stateSuccess",
    color: "$stateSuccess",
  },
  warning: {
    backgroundColor: "$stateWarningSurface",
    borderColor: "$stateWarning",
    color: "$stateWarning",
  },
} as const;

// "#0B0C0E" + 0.15 → "rgba(11,12,14,0.15)", the same formula CSS `color-mix(in srgb, X 15%,
// transparent)` resolves to (shared with Text's halo via the module-level `withAlpha` below).
const squareBadgeTone = (theme: ReturnType<typeof useTheme>) =>
  ({
    // The only square tone with a flat, non-tinted background — the legacy ledger's "neutral".
    neutral: { backgroundColor: theme.surfaceElevated.val, color: theme.contentSecondary.val },
    tune: { backgroundColor: withAlpha(theme.stateInfo.val, 0.15), color: theme.stateInfo.val },
    lock: { backgroundColor: withAlpha(theme.stateSuccess.val, 0.15), color: theme.stateSuccess.val },
    caution: { backgroundColor: withAlpha(theme.stateWarning.val, 0.15), color: theme.stateWarning.val },
    // `danger`/`stateDanger` already resolves to the AA-safe onair-300 stop; the tint itself uses
    // the stronger base onair red, same split as the legacy `bg-onair-tint-15 text-onair-300`.
    onair: { backgroundColor: withAlpha(theme.guideOnAir.val, 0.15), color: theme.stateDanger.val },
    // The brand chroma stops are theme-invariant (same as `guide.onAir`), so the base suggest tint
    // comes straight from `brandChroma` rather than a theme entry.
    suggest: { backgroundColor: withAlpha(brandChroma[4], 0.15), color: theme.accentSuggest.val },
    signal: { backgroundColor: withAlpha(theme.actionPrimary.val, 0.15), color: theme.actionPrimary.val },
  }) as const;

const Badge = ({ children, density = "pointer", shape = "pill", tone = "neutral", ...props }: BadgeProps) => {
  const theme = useTheme();

  if (shape === "square") {
    const squareTones = squareBadgeTone(theme);
    const colors = tone in squareTones ? squareTones[tone as keyof typeof squareTones] : squareTones.neutral;
    return (
      <View
        {...props}
        alignItems="center"
        alignSelf="flex-start"
        backgroundColor={colors.backgroundColor}
        borderRadius={4}
        flexShrink={0}
        paddingHorizontal={6}
        paddingVertical={2}
      >
        <TamaguiText
          color={colors.color}
          fontFamily="$data"
          fontSize={11}
          fontWeight="500"
          letterSpacing={0.275}
          lineHeight={14}
          textTransform="uppercase"
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
      alignSelf="flex-start"
      backgroundColor={colors.backgroundColor}
      borderColor={colors.borderColor}
      borderRadius="$round"
      borderWidth={1}
      flexShrink={0}
      paddingHorizontal="$inline"
      paddingVertical={4}
    >
      <TamaguiText
        color={colors.color}
        fontFamily="$data"
        fontSize={typography[density].metadata.size}
        fontWeight="600"
        lineHeight={typography[density].metadata.lineHeight}
      >
        {children}
      </TamaguiText>
    </View>
  );
};

type ArtworkState = "error" | "loading" | "missing" | "ready";
type ArtworkFrameProps = Omit<ComponentProps<typeof Surface>, "children"> & {
  children?: ReactNode;
  density?: Density;
  state: ArtworkState;
};

const artworkCopy: Record<Exclude<ArtworkState, "ready">, string> = {
  error: "Artwork unavailable",
  loading: "Loading artwork",
  missing: "No artwork",
};

const ArtworkFrame = ({ children, density = "pointer", state, ...props }: ArtworkFrameProps) => (
  <Surface
    {...props}
    alignItems="center"
    aspectRatio={16 / 9}
    backgroundColor="$artworkPlaceholder"
    justifyContent="center"
    overflow="hidden"
  >
    {state === "ready" && children ? (
      children
    ) : (
      <Text density={density} textRole="metadata">
        {artworkCopy[state === "ready" ? "missing" : state]}
      </Text>
    )}
  </Surface>
);

type ProgressTone = "artwork" | "live" | "lock" | "onair" | "primary" | "tune";
type ProgressTrackProps = Omit<ComponentProps<typeof View>, "children"> & {
  /**
   * How far along, 0-100. Omit it for an INDETERMINATE bar: when `label` is set, this is the ARIA
   * contract (no `aria-valuenow` means "busy"), so never pass 0 to mean "no measurement yet".
   */
  percent?: number;
  // `artwork` is drawn across a still (#1659 web mock cards): a light fill on a translucent track.
  tone?: ProgressTone;
  // `square` runs edge to edge along a still's foot (the web mock's On now card).
  ends?: "round" | "square";
  /**
   * Accessible name. Omit it to keep the bar decorative (an adjacent caller already names it);
   * set it to turn the bar into a real `role="progressbar"` with the aria-value trio (§970 PR B).
   */
  label?: string;
  reducedMotion?: boolean;
};

const progressFillVal = (theme: ReturnType<typeof useTheme>) =>
  ({
    artwork: theme.contentPrimary.val,
    live: theme.stateLive.val,
    lock: theme.stateSuccess.val,
    onair: theme.guideOnAir.val,
    primary: theme.actionPrimary.val,
    tune: theme.stateInfo.val,
  }) as const;

const ProgressTrack = ({
  ends = "round",
  height = 4,
  label,
  percent,
  reducedMotion,
  tone = "primary",
  ...props
}: ProgressTrackProps) => {
  const theme = useTheme();
  const determinate = percent != null;
  const bounded = determinate ? Math.max(0, Math.min(100, percent)) : 100;
  const borderRadius = ends === "round" ? "$round" : 0;
  const prefersReducedMotion = useReducedMotionPreference(reducedMotion);
  const pulse = useRef(new Animated.Value(1)).current;

  useEffect(() => {
    if (determinate || prefersReducedMotion !== false) {
      pulse.stopAnimation();
      pulse.setValue(determinate ? 1 : 0.6);
      return undefined;
    }
    pulse.setValue(1);
    const animation = Animated.loop(
      Animated.sequence([
        Animated.timing(pulse, {
          duration: 1000,
          easing: Easing.inOut(Easing.quad),
          toValue: 0.5,
          useNativeDriver: useNativeAnimationDriver,
        }),
        Animated.timing(pulse, {
          duration: 1000,
          easing: Easing.inOut(Easing.quad),
          toValue: 1,
          useNativeDriver: useNativeAnimationDriver,
        }),
      ]),
    );
    animation.start();
    return () => animation.stop();
  }, [determinate, prefersReducedMotion, pulse]);

  return (
    <View
      {...props}
      {...(label
        ? {
            "aria-label": label,
            role: "progressbar",
            ...(determinate ? { "aria-valuemax": 100, "aria-valuemin": 0, "aria-valuenow": bounded } : {}),
          }
        : {})}
      backgroundColor={tone === "artwork" ? "$artworkProgressTrack" : "$surfaceCanvas"}
      borderRadius={borderRadius}
      height={height}
      overflow="hidden"
    >
      <Animated.View
        style={{
          backgroundColor: progressFillVal(theme)[tone],
          borderRadius: typeof borderRadius === "number" ? borderRadius : 999,
          height: "100%",
          opacity: pulse,
          width: `${bounded}%`,
        }}
      />
    </View>
  );
};

type StatusTone = "attention" | "error" | "live" | "notice" | "off" | "ok" | "pending" | "progress" | "warn";
type StatusDotProps = {
  /**
   * What the dot MEANS, for assistive tech — required, because a bare coloured circle conveys
   * nothing without it. Pass "" to opt out explicitly, for a dot beside text that already says it.
   */
  label: string;
  reducedMotion?: boolean;
  /** 8px by default; `compact` is 6px (a tighter row of dots). */
  size?: "compact" | "default";
  tone: StatusTone;
};

const statusDotTone = (theme: ReturnType<typeof useTheme>) =>
  ({
    live: theme.guideOnAir.val,
    error: theme.guideOnAir.val,
    pending: theme.guideReconciling.val,
    ok: theme.stateSuccess.val,
    warn: theme.stateWarning.val,
    off: theme.actionDisabled.val,
    attention: theme.accentSuggest.val,
    progress: theme.stateInfo.val,
    notice: theme.actionPrimary.val,
  }) as const;

/**
 * The pulse is reserved for `live` alone (§970 PR B, carried over from the legacy StatusDot):
 * motion means "happening right now", so `error` shares `live`'s colour but never animates — a
 * job that failed hours ago has nothing happening, and pulsing would say otherwise.
 */
const StatusDot = ({ label, reducedMotion, size = "default", tone }: StatusDotProps) => {
  const theme = useTheme();
  const prefersReducedMotion = useReducedMotionPreference(reducedMotion);
  const pulse = useRef(new Animated.Value(1)).current;
  const pulses = tone === "live";

  useEffect(() => {
    if (!pulses || prefersReducedMotion !== false) {
      pulse.stopAnimation();
      pulse.setValue(1);
      return undefined;
    }
    pulse.setValue(1);
    const animation = Animated.loop(
      Animated.sequence([
        Animated.timing(pulse, {
          duration: 1000,
          easing: Easing.inOut(Easing.quad),
          toValue: 0.5,
          useNativeDriver: useNativeAnimationDriver,
        }),
        Animated.timing(pulse, {
          duration: 1000,
          easing: Easing.inOut(Easing.quad),
          toValue: 1,
          useNativeDriver: useNativeAnimationDriver,
        }),
      ]),
    );
    animation.start();
    return () => animation.stop();
  }, [pulses, prefersReducedMotion, pulse]);

  const dimension = size === "compact" ? 6 : 8;
  return (
    <Animated.View
      {...(label ? { role: "img", "aria-label": label } : { "aria-hidden": true })}
      style={{
        backgroundColor: statusDotTone(theme)[tone],
        borderRadius: 999,
        flexShrink: 0,
        height: dimension,
        opacity: pulse,
        width: dimension,
      }}
    />
  );
};

export type {
  ArtworkState,
  BadgeShape,
  BadgeTone,
  ProgressTone,
  ScreenProps,
  StatusTone,
  TextProps,
  TextTone,
};
export { ArtworkFrame, Badge, FocusSurface, ProgressTrack, Screen, StatusDot, Surface, Text };
