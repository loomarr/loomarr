import { isWeb, styled, useTheme, View } from "@tamagui/core";
import type { ComponentProps, ReactNode } from "react";
import { useEffect, useRef } from "react";
import { Animated, Easing } from "react-native";

import { themeColors } from "../config";
import { useNativeAnimationDriver } from "../motion/animation-driver";
import { useReducedMotionPreference } from "../motion/use-reduced-motion";
import { Text, type TextTone, textTones } from "../text";
import { brandChroma, type Density } from "../tokens";
import { resolveViewportInsets, useViewportInsets } from "../viewport";

const ScreenFrame = styled(View, {
  name: "LoomarrScreen",
  flex: 1,
  width: "100%",
  minHeight: isWeb ? "100vh" : "100%",
  backgroundColor: "surfaceCanvas",
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
  borderColor: "borderDecorative",
  borderWidth: 1,
  borderRadius: "card",
  variants: {
    level: {
      canvas: {
        backgroundColor: "surfaceCanvas",
      },
      raised: {
        backgroundColor: "surfaceRaised",
      },
      elevated: {
        backgroundColor: "surfaceElevated",
      },
      overlay: {
        backgroundColor: "surfaceOverlay",
        borderRadius: "overlay",
      },
      focus: {
        backgroundColor: "surfaceFocus",
        borderColor: "actionFocus",
      },
    },
  } as const,
  defaultVariants: {
    level: "raised",
  },
});

const FocusSurface = styled(Surface, {
  name: "LoomarrFocusSurface",
  borderWidth: 2,
  borderColor: "transparent",
  variants: {
    focused: {
      false: {
        backgroundColor: "surfaceRaised",
        borderColor: "transparent",
      },
      true: {
        backgroundColor: "surfaceFocus",
        borderColor: "actionFocus",
      },
    },
  } as const,
  defaultVariants: {
    focused: false,
  },
});

// The monogram ident's colours (#3011): a 14% wash of the brand hue, with the letters in that hue's
// on-tint text stop (the web's `-300` stops in dark; each theme's own text stop in light) rather
// than the full hue, which fails AA on its own wash. `tone` is how a Text draws the letters, so
// each theme resolves its own stop. The keys are `@loomarr/core/guide`'s ident hues.
const CHANNEL_IDENT_WASH_PERCENT = 14;
const channelIdentHues = {
  tune: { wash: brandChroma[3], tone: "identTune" },
  suggest: { wash: brandChroma[4], tone: "identSuggest" },
  signal: { wash: brandChroma[0], tone: "identSignal" },
  onair: { wash: brandChroma[5], tone: "identOnair" },
} as const satisfies Record<string, { wash: string; tone: TextTone }>;

type ChannelIdentColorHue = keyof typeof channelIdentHues;

/** Each ident hue's wash and letter colour as concrete values in one theme (for the contrast test). */
const channelIdentColors = (mode: "dark" | "light") => {
  const colors = themeColors(mode);
  return Object.fromEntries(
    Object.entries(channelIdentHues).map(([hue, { wash, tone }]) => [
      hue,
      { text: colors[textTones[tone]], wash },
    ]),
  ) as Record<ChannelIdentColorHue, { text: string; wash: string }>;
};

type ArtworkState = "error" | "loading" | "missing" | "ready";
/**
 * The frame's shape: `16:9` for a still, a channel or a programme (the default); `2:3` for a film or
 * series poster.
 */
type ArtworkRatio = "16:9" | "2:3";
const artworkRatios: Record<ArtworkRatio, number> = { "16:9": 16 / 9, "2:3": 2 / 3 };

type ArtworkFrameProps = Omit<ComponentProps<typeof Surface>, "aspectRatio" | "children"> & {
  children?: ReactNode;
  density?: Density;
  ratio?: ArtworkRatio;
  state: ArtworkState;
};

const artworkCopy: Record<Exclude<ArtworkState, "ready">, string> = {
  error: "Artwork unavailable",
  loading: "Loading artwork",
  missing: "No artwork",
};

const ArtworkFrame = ({
  children,
  density = "pointer",
  ratio = "16:9",
  state,
  ...props
}: ArtworkFrameProps) => (
  <Surface
    {...props}
    alignItems="center"
    aspectRatio={artworkRatios[ratio]}
    backgroundColor="artworkPlaceholder"
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

/**
 * `elapsed`, the default, is time gone in an airing: foreground, because amber is focus and the
 * primary action only (#1909 Q3). A download or preparation passes `tune`.
 */
type ProgressTone = "artwork" | "elapsed" | "live" | "lock" | "onair" | "tune";
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
    elapsed: theme.contentPrimary.val,
    live: theme.stateLive.val,
    lock: theme.stateSuccess.val,
    onair: theme.guideOnAir.val,
    tune: theme.stateInfo.val,
  }) as const;

const ProgressTrack = ({
  ends = "round",
  height = 4,
  label,
  percent,
  reducedMotion,
  tone = "elapsed",
  ...props
}: ProgressTrackProps) => {
  const theme = useTheme();
  const determinate = percent != null;
  const bounded = determinate ? Math.max(0, Math.min(100, percent)) : 100;
  const borderRadius = ends === "round" ? "round" : 0;
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
      backgroundColor={tone === "artwork" ? "artworkProgressTrack" : "surfaceCanvas"}
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

export type { ArtworkRatio, ArtworkState, ChannelIdentColorHue, ProgressTone, ScreenProps };
export {
  ArtworkFrame,
  CHANNEL_IDENT_WASH_PERCENT,
  channelIdentColors,
  channelIdentHues,
  FocusSurface,
  ProgressTrack,
  Screen,
  Surface,
};
