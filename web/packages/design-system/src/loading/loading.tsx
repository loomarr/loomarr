import { useTheme, View } from "@tamagui/core";
import type { ComponentProps } from "react";
import { useEffect, useMemo, useRef } from "react";
import { Animated, Easing } from "react-native";

import { Icon, type IconSize, type IconTone, icons } from "../icon";
import { useNativeAnimationDriver } from "../motion/animation-driver";
import { useReducedMotionPreference } from "../motion/use-reduced-motion";
import { Text } from "../text";
import { brandChroma, type Density, semanticMotion } from "../tokens";

type AccessibleLoading = { accessibilityLabel: string; decorative?: false };
type DecorativeLoading = { accessibilityLabel?: never; decorative: true };

type ActivityIndicatorProps = (AccessibleLoading | DecorativeLoading) & {
  reducedMotion?: boolean;
  size?: IconSize;
  tone?: IconTone;
};

const activityIndicatorMotion = {
  duration: 900,
} as const;

const ActivityIndicator = ({
  accessibilityLabel,
  decorative,
  reducedMotion,
  size = "default",
  tone = "primary",
}: ActivityIndicatorProps) => {
  const prefersReducedMotion = useReducedMotionPreference(reducedMotion);
  const rotation = useRef(new Animated.Value(0)).current;

  useEffect(() => {
    if (prefersReducedMotion !== false) {
      rotation.stopAnimation();
      rotation.setValue(0);
      return undefined;
    }

    const animation = Animated.loop(
      Animated.timing(rotation, {
        duration: activityIndicatorMotion.duration,
        easing: Easing.linear,
        toValue: 1,
        useNativeDriver: useNativeAnimationDriver,
      }),
    );
    animation.start();
    return () => animation.stop();
  }, [prefersReducedMotion, rotation]);

  return (
    <Animated.View
      aria-label={decorative ? undefined : accessibilityLabel}
      aria-hidden={decorative || undefined}
      role={decorative ? undefined : "progressbar"}
      style={{
        alignSelf: "flex-start",
        transform: [
          {
            rotate: rotation.interpolate({ inputRange: [0, 1], outputRange: ["0deg", "360deg"] }),
          },
        ],
      }}
    >
      <Icon decorative glyph={icons.loading} size={size} tone={tone} />
    </Animated.View>
  );
};

const signalLoaderMotion = {
  duration: 1600,
  stagger: 90,
  bars: 9,
} as const;

const signalDimensions = {
  pointer: { barGap: 4, barWidth: 4, labelMargin: 10, maxHeight: 28 },
  touch: { barGap: 5, barWidth: 5, labelMargin: 12, maxHeight: 34 },
  tv: { barGap: 7, barWidth: 7, labelMargin: 16, maxHeight: 48 },
} as const;

const barShape = [0.42, 0.64, 0.86, 0.58, 1, 0.72, 0.9, 0.52, 0.76] as const;
const barNames = ["one", "two", "three", "four", "five", "six", "seven", "eight", "nine"] as const;

type SignalLoaderProps = Omit<ComponentProps<typeof View>, "children"> & {
  density?: Density;
  detail?: string;
  /** `null` draws the bars alone, for a caller that sets its own readout (the TV switch, #1627). */
  label?: string | null;
  reducedMotion?: boolean;
};

const SignalLoader = ({
  accessibilityLabel,
  density = "pointer",
  detail,
  label = "TUNING IN",
  reducedMotion,
  ...props
}: SignalLoaderProps) => {
  const theme = useTheme();
  const prefersReducedMotion = useReducedMotionPreference(reducedMotion);
  const phase = useRef(new Animated.Value(0)).current;
  const dimensions = signalDimensions[density];

  useEffect(() => {
    if (prefersReducedMotion !== false) {
      phase.stopAnimation();
      phase.setValue(1);
      return undefined;
    }

    phase.setValue(0);

    const animation = Animated.loop(
      Animated.sequence([
        Animated.timing(phase, {
          duration: signalLoaderMotion.duration,
          easing: Easing.inOut(Easing.quad),
          toValue: 1,
          useNativeDriver: false,
        }),
        Animated.delay(semanticMotion.focus),
        Animated.timing(phase, { duration: 0, toValue: 0, useNativeDriver: false }),
      ]),
    );
    animation.start();
    return () => animation.stop();
  }, [phase, prefersReducedMotion]);

  const barStyles = useMemo(
    () =>
      barShape.map((height, index) => {
        const start = (index * signalLoaderMotion.stagger) / signalLoaderMotion.duration;
        const locked = Math.min(start + 0.26, 0.84);
        return {
          backgroundColor: phase.interpolate({
            inputRange: [0, start, locked, 1],
            outputRange: [
              theme.actionDisabled.val,
              theme.actionDisabled.val,
              theme.actionPrimary.val,
              theme.actionPrimary.val,
            ],
          }),
          height: Math.round(dimensions.maxHeight * height),
          opacity: phase.interpolate({
            inputRange: [0, start, locked, 1],
            outputRange: [0.42, 0.42, 1, 1],
          }),
          transform: [
            {
              scaleY: phase.interpolate({
                inputRange: [0, start, locked, 1],
                outputRange: [0.58, 0.58, 1, 1],
              }),
            },
          ],
          width: dimensions.barWidth,
        };
      }),
    [dimensions, phase, theme.actionDisabled.val, theme.actionPrimary.val],
  );

  return (
    <View
      {...props}
      aria-label={accessibilityLabel ?? [label, detail].filter(Boolean).join(", ")}
      role="progressbar"
      alignItems="center"
    >
      <View alignItems="flex-end" flexDirection="row" gap={dimensions.barGap} height={dimensions.maxHeight}>
        {barStyles.map((style, index) => (
          <Animated.View aria-hidden key={barNames[index]} style={style} />
        ))}
      </View>
      {label === null ? null : (
        <Text marginTop={dimensions.labelMargin} density={density} textRole="label">
          {label}
        </Text>
      )}
      {detail ? (
        <Text marginTop="inline" density={density} textRole="metadata">
          {detail}
        </Text>
      ) : null}
    </View>
  );
};

type SkeletonShape = "circle" | "line" | "media";
type SkeletonProps = {
  height?: number | `${number}%`;
  reducedMotion?: boolean;
  shape?: SkeletonShape;
  width?: number | `${number}%`;
};

const skeletonMotion = {
  duration: 1100,
  opacity: { high: 0.72, low: 0.34 },
} as const;

const Skeleton = ({ height, reducedMotion, shape = "line", width }: SkeletonProps) => {
  const theme = useTheme();
  const prefersReducedMotion = useReducedMotionPreference(reducedMotion);
  const pulse = useRef(new Animated.Value(skeletonMotion.opacity.low)).current;

  useEffect(() => {
    if (prefersReducedMotion !== false) {
      pulse.stopAnimation();
      pulse.setValue(skeletonMotion.opacity.high);
      return undefined;
    }

    const animation = Animated.loop(
      Animated.sequence([
        Animated.timing(pulse, {
          duration: skeletonMotion.duration,
          easing: Easing.inOut(Easing.quad),
          toValue: skeletonMotion.opacity.high,
          useNativeDriver: useNativeAnimationDriver,
        }),
        Animated.timing(pulse, {
          duration: skeletonMotion.duration,
          easing: Easing.inOut(Easing.quad),
          toValue: skeletonMotion.opacity.low,
          useNativeDriver: useNativeAnimationDriver,
        }),
      ]),
    );
    animation.start();
    return () => animation.stop();
  }, [prefersReducedMotion, pulse]);

  const resolvedHeight = height ?? (shape === "media" ? 180 : shape === "circle" ? 40 : 14);
  const resolvedWidth = width ?? (shape === "circle" ? resolvedHeight : "100%");

  return (
    <Animated.View
      aria-hidden
      style={{
        backgroundColor: theme.surfaceElevated.val,
        borderRadius: shape === "circle" ? 999 : shape === "media" ? 16 : 999,
        height: resolvedHeight,
        opacity: pulse,
        width: resolvedWidth,
      }}
    />
  );
};

type LoadingStripProps = (AccessibleLoading | DecorativeLoading) & {
  density?: Density;
  reducedMotion?: boolean;
  /** Pixels, or a percentage of the container. A page load spans its column; a line of text takes a short one. */
  width?: number | `${number}%`;
};

const loadingStripMotion = {
  duration: 1400,
  // Each segment's lit window, as a fraction of the cycle; the windows overlap so the light travels.
  window: 0.3,
  opacity: { lit: 1, rest: 0.35 },
} as const;

const loadingStripHeight: Record<Density, number> = { pointer: 3, touch: 4, tv: 6 };

// The brand mark's seven bars, in its order: signal · caution · lock · tune · suggest · onair · grey.
const stripSegments = brandChroma.map((color, index) => ({ color, key: `bar-${index}` }));

// LoadingStrip is the wait for a page or a section (#2789): the brand's colour bars as a thin strip,
// lit one after another. Opacity only, no transform: the only movement the product makes outside
// the tune and sheets is a fade. With reduced motion it is the still bars. Buttons keep
// ActivityIndicator, the small spinner beside "Saving…".
const LoadingStrip = ({
  accessibilityLabel,
  decorative,
  density = "pointer",
  reducedMotion,
  width = 120,
}: LoadingStripProps) => {
  const prefersReducedMotion = useReducedMotionPreference(reducedMotion);
  const phase = useRef(new Animated.Value(0)).current;

  useEffect(() => {
    if (prefersReducedMotion !== false) {
      phase.stopAnimation();
      phase.setValue(0);
      return undefined;
    }

    const animation = Animated.loop(
      Animated.timing(phase, {
        duration: loadingStripMotion.duration,
        easing: Easing.linear,
        toValue: 1,
        useNativeDriver: useNativeAnimationDriver,
      }),
    );
    animation.start();
    return () => animation.stop();
  }, [phase, prefersReducedMotion]);

  const opacities = useMemo(() => {
    const { lit, rest } = loadingStripMotion.opacity;
    const span = 1 - loadingStripMotion.window;
    return stripSegments.map((_, index) => {
      const start = (index / (stripSegments.length - 1)) * span;
      return phase.interpolate({
        extrapolate: "clamp",
        inputRange: [start, start + loadingStripMotion.window / 2, start + loadingStripMotion.window],
        outputRange: [rest, lit, rest],
      });
    });
  }, [phase]);

  const height = loadingStripHeight[density];
  return (
    <View
      aria-hidden={decorative || undefined}
      aria-label={decorative ? undefined : accessibilityLabel}
      borderRadius={height / 2}
      flexDirection="row"
      height={height}
      overflow="hidden"
      role={decorative ? undefined : "progressbar"}
      width={width}
    >
      {stripSegments.map(({ color, key }, index) => (
        <Animated.View
          aria-hidden
          key={key}
          style={{
            backgroundColor: color,
            flex: 1,
            opacity: prefersReducedMotion === false ? opacities[index] : loadingStripMotion.opacity.lit,
          }}
        />
      ))}
    </View>
  );
};

export type { ActivityIndicatorProps, LoadingStripProps, SignalLoaderProps, SkeletonProps, SkeletonShape };
export {
  ActivityIndicator,
  activityIndicatorMotion,
  LoadingStrip,
  loadingStripMotion,
  SignalLoader,
  Skeleton,
  signalLoaderMotion,
  skeletonMotion,
};
