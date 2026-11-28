import type { TuneBarsPhase, TuneCover } from "@loomarr/core/tune";
import { TUNE_BURST_MS, TUNE_FLIP_MS, TUNE_UNCOVER_MS } from "@loomarr/core/tune";
import {
  AnalogSnow,
  brandChroma,
  playerChrome,
  semanticColors,
  ThemeScope,
  useReducedMotionPreference,
} from "@loomarr/design-system";
import { useEffect, useRef } from "react";
import { Animated, Easing, View } from "react-native";

// The Cross's tune (frontend-design §9.4): dim static over the picture and the seven bars along the
// crossing. Both parts take what `tuneView` says and draw it. They decide nothing and play no sound.

// How much of the picture each cover hides. Even over a white frame what shows is a dim grey.
const DIM: Record<TuneCover, number> = { burst: 0.88, cold: 0.8, fading: 0, none: 0, wash: 0.88 };
const FADE_MS = TUNE_UNCOVER_MS - TUNE_BURST_MS;
// Reduced motion's crossfade, the system's fade (§2.4).
const CROSSFADE_MS = 120;

// The brand chroma is the seven bars, in order. Waiting, they alternate two greys.
const WAITING = brandChroma.map((_, i) =>
  i % 2 === 0 ? semanticColors.content.secondary : semanticColors.action.disabled,
);
const BAR_HEIGHT = 3;
// The share of a run spent crossing; the rest fades the bars away.
const CROSSED = 0.7;

const fill = { bottom: 0, left: 0, position: "absolute", right: 0, top: 0 } as const;

type TuneStaticProps = {
  cover: TuneCover;
  /** For story baselines only: draws the static without its flicker. Product callers leave it unset. */
  holdSnow?: boolean;
  reducedMotion?: boolean;
};

/**
 * The cover over the picture. Fills its parent, takes no touch and is hidden from assistive
 * technology. It stays mounted while clear. Under reduced motion it is a plain crossfade of the
 * darkened picture with no static.
 */
const TuneStatic = ({ cover, holdSnow = false, reducedMotion }: TuneStaticProps) => {
  // An unknown preference (null, until the platform answers) counts as reduced: no static is drawn
  // for someone who may have asked for none.
  const still = useReducedMotionPreference(reducedMotion) !== false;
  const level = useRef(new Animated.Value(still ? 0 : DIM[cover])).current;

  useEffect(() => {
    // Static cuts in and out: the picture is covered at once.
    const duration = still ? CROSSFADE_MS : cover === "fading" ? FADE_MS : 0;
    if (duration === 0) {
      level.setValue(DIM[cover]);
      return undefined;
    }
    const fade = Animated.timing(level, { duration, toValue: DIM[cover], useNativeDriver: false });
    fade.start();
    return () => fade.stop();
  }, [cover, level, still]);

  return (
    <Animated.View
      aria-hidden
      pointerEvents="none"
      style={{ ...fill, opacity: level }}
      testID={`tune-static-${cover}`}
    >
      <View style={{ ...fill, backgroundColor: playerChrome.surface }} />
      {still ? null : (
        // The picture's plane is dark in either theme, so the snow is never a light wash. The snow is
        // hundreds of views, so it stays mounted between covers, hidden and without its flicker.
        // The snow is told outright, so it never asks the platform and its flicker starts with the cover.
        <ThemeScope theme="dark">
          <AnalogSnow reducedMotion={holdSnow || cover === "none"} />
        </ThemeScope>
      )}
    </Animated.View>
  );
};

type TuneBarsProps = {
  bars: TuneBarsPhase;
  reducedMotion?: boolean;
  /** `tuneView`'s run count: a new one runs the bars again. */
  run: number;
};

/**
 * The seven bars, a thin strip as wide as its parent; the host places it along the crossing.
 * Under reduced motion the run is not drawn, and waiting and locked bars hold still.
 */
const TuneBars = ({ bars, reducedMotion, run }: TuneBarsProps) => {
  const still = useReducedMotionPreference(reducedMotion) !== false;
  const progress = useRef(new Animated.Value(0)).current;
  const running = bars === "run" && !still;

  // biome-ignore lint/correctness/useExhaustiveDependencies: `run` runs the bars again
  useEffect(() => {
    if (!running) return undefined;
    progress.setValue(0);
    const animation = Animated.timing(progress, {
      duration: TUNE_FLIP_MS,
      easing: Easing.linear,
      toValue: 1,
      useNativeDriver: false,
    });
    animation.start();
    return () => animation.stop();
  }, [progress, run, running]);

  if (bars === "none" || (bars === "run" && still)) return null;
  const colours = bars === "waiting" ? WAITING : brandChroma;

  return (
    <Animated.View
      aria-hidden
      pointerEvents="none"
      style={{
        flexDirection: "row",
        height: BAR_HEIGHT,
        opacity: running ? progress.interpolate({ inputRange: [0, CROSSED, 1], outputRange: [1, 1, 0] }) : 1,
        width: "100%",
      }}
      testID={`tune-bars-${bars}`}
    >
      {colours.map((colour, i) => {
        const arrives = (i / colours.length) * CROSSED;
        return (
          <Animated.View
            // biome-ignore lint/suspicious/noArrayIndexKey: seven fixed positions; colours repeat
            key={i}
            style={{
              backgroundColor: colour,
              flex: 1,
              opacity: running
                ? progress.interpolate({
                    extrapolate: "clamp",
                    inputRange: [arrives, arrives + CROSSED / colours.length],
                    outputRange: [0, 1],
                  })
                : 1,
            }}
          />
        );
      })}
    </Animated.View>
  );
};

export type { TuneBarsProps, TuneStaticProps };
export { TuneBars, TuneStatic };
