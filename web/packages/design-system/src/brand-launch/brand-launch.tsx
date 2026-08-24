import { useTheme, View } from "@tamagui/core";
import { useEffect, useRef } from "react";
import { Animated, Easing } from "react-native";
import { BrandWordmark } from "../brand/brand";
import { useNativeAnimationDriver } from "../motion/animation-driver";
import { useReducedMotionPreference } from "../motion/use-reduced-motion";
import type { Density } from "../tokens";
import { brandChroma } from "../tokens";

const brandLaunchMotion = {
  segmentDuration: 340,
  segmentStagger: 40,
  wordDuration: 400,
  wordDelay: 220,
  taglineDelay: 320,
} as const;

type BrandLaunchProps = {
  density?: Density;
  onFinished?: () => void;
  reducedMotion?: boolean;
  showTagline?: boolean;
};

const launchSizes = {
  pointer: { barHeight: 14, barWidth: 200, minHeight: 480, word: "large" },
  touch: { barHeight: 11, barWidth: 160, minHeight: 560, word: "medium" },
  tv: { barHeight: 22, barWidth: 320, minHeight: 540, word: "large" },
} as const;

const BrandLaunch = ({
  density = "pointer",
  onFinished,
  reducedMotion,
  showTagline = true,
}: BrandLaunchProps) => {
  const theme = useTheme();
  const sizes = launchSizes[density];
  const systemReducedMotion = useReducedMotionPreference(reducedMotion);
  const segmentProgress = useRef(brandChroma.map(() => new Animated.Value(reducedMotion ? 1 : 0))).current;
  const wordProgress = useRef(new Animated.Value(reducedMotion ? 1 : 0)).current;
  const taglineProgress = useRef(new Animated.Value(reducedMotion ? 1 : 0)).current;
  const finishedRef = useRef(onFinished);
  finishedRef.current = onFinished;

  useEffect(() => {
    if (systemReducedMotion === null) return undefined;

    const values = [...segmentProgress, wordProgress, taglineProgress];
    values.forEach((value) => {
      value.stopAnimation();
    });

    if (systemReducedMotion) {
      segmentProgress.forEach((value) => {
        value.setValue(1);
      });
      wordProgress.setValue(1);
      taglineProgress.setValue(1);
      finishedRef.current?.();
      return undefined;
    }

    values.forEach((value) => {
      value.setValue(0);
    });
    const easeOut = Easing.bezier(0.16, 1, 0.3, 1);
    const identity = Animated.parallel([
      Animated.stagger(
        brandLaunchMotion.segmentStagger,
        segmentProgress.map((value) =>
          Animated.timing(value, {
            duration: brandLaunchMotion.segmentDuration,
            easing: easeOut,
            toValue: 1,
            useNativeDriver: useNativeAnimationDriver,
          }),
        ),
      ),
      Animated.sequence([
        Animated.delay(brandLaunchMotion.wordDelay),
        Animated.timing(wordProgress, {
          duration: brandLaunchMotion.wordDuration,
          easing: easeOut,
          toValue: 1,
          useNativeDriver: useNativeAnimationDriver,
        }),
      ]),
      Animated.sequence([
        Animated.delay(brandLaunchMotion.taglineDelay),
        Animated.timing(taglineProgress, {
          duration: brandLaunchMotion.wordDuration,
          easing: easeOut,
          toValue: 1,
          useNativeDriver: useNativeAnimationDriver,
        }),
      ]),
    ]);
    identity.start(({ finished }) => {
      if (finished) finishedRef.current?.();
    });

    return () => {
      identity.stop();
    };
  }, [segmentProgress, systemReducedMotion, taglineProgress, wordProgress]);

  return (
    <View
      aria-label="Loomarr is starting"
      role="img"
      alignItems="center"
      backgroundColor="$surfaceCanvas"
      flex={1}
      justifyContent="center"
      minHeight={sizes.minHeight}
      overflow="hidden"
      position="relative"
      width="100%"
    >
      <View alignItems="center" gap={density === "tv" ? 18 : 14}>
        <View
          borderRadius={2}
          flexDirection="row"
          height={sizes.barHeight}
          overflow="hidden"
          width={sizes.barWidth}
        >
          {brandChroma.map((color, index) => (
            <Animated.View
              key={color}
              style={{
                backgroundColor: color,
                flex: 1,
                opacity: segmentProgress[index]!,
                transform: [
                  {
                    scaleY: segmentProgress[index]!.interpolate({
                      inputRange: [0, 1],
                      outputRange: [0.15, 1],
                    }),
                  },
                ],
              }}
            />
          ))}
        </View>

        <Animated.View
          style={{
            opacity: wordProgress,
            transform: [
              { translateY: wordProgress.interpolate({ inputRange: [0, 1], outputRange: [5, 0] }) },
            ],
          }}
        >
          <BrandWordmark size={sizes.word} />
        </Animated.View>

        {showTagline ? (
          <Animated.Text
            style={{
              color: theme.contentSecondary.val,
              fontFamily: "monospace",
              fontSize: density === "tv" ? 18 : 12,
              opacity: taglineProgress,
              transform: [
                { translateY: taglineProgress.interpolate({ inputRange: [0, 1], outputRange: [5, 0] }) },
              ],
            }}
          >
            always something on
          </Animated.Text>
        ) : null}
      </View>
    </View>
  );
};

export type { BrandLaunchProps };
export { BrandLaunch, brandLaunchMotion };
