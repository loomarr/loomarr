import { useTheme, View } from "@tamagui/core";
import { useEffect, useMemo, useRef } from "react";
import { Animated, Easing } from "react-native";
import Svg, { Defs, Pattern, Rect } from "react-native-svg";

import { useNativeAnimationDriver } from "../motion/animation-driver";
import { useReducedMotionPreference } from "../motion/use-reduced-motion";

// The grain: cells taller than wide, like the web snow's noise (1.6 × 2.8), in one seeded tile
// the pattern repeats. 24 × 24 cells is enough that the repeat never reads as a grid at TV size.
const CELL = { height: 3, width: 2 } as const;
const TILE_CELLS = 24;
const TILE = { height: CELL.height * TILE_CELLS, width: CELL.width * TILE_CELLS } as const;

// Stepped jumps, held for a quarter of the web keyframe's 0.45 s each: real static flickers, it
// doesn't drift. Offsets in dp, inside the oversize, so an edge never shows.
const FLICKER_OFFSETS = [
  [0, 0],
  [-13, 8],
  [7, -11],
  [-9, -5],
  [11, 7],
] as const;
const FLICKER_STEP_MS = 112;
const OVERSIZE = 16;

// mulberry32: a tiny seeded PRNG, so a still frame is the same every render and on every client.
const seeded = (seed: number) => {
  let a = seed >>> 0;
  return () => {
    a = (a + 0x6d2b79f5) >>> 0;
    let t = a;
    t = Math.imul(t ^ (t >>> 15), t | 1);
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
};

const clamp01 = (n: number) => Math.min(1, Math.max(0, n));
const channel = (n: number) => Math.round(clamp01(n) * 255);

// One cell's colour: the web snow's gain as a grey ramp (1.2·n − 0.34), blue lifted +0.02 like
// the web matrix's blue row, so the snow is faintly cold rather than dead grey (#1627).
const cellColor = (n: number) => {
  const grey = 1.2 * n - 0.34;
  return `rgb(${channel(grey)},${channel(grey)},${channel(grey + 0.02)})`;
};

type AnalogSnowProps = {
  reducedMotion?: boolean;
  seed?: number;
};

/**
 * AnalogSnow — the channel-switch wash (#1627, the web's B4 `TvStatic` wash): analog snow under the
 * canvas at 90% with a dark scanline every third unit, so a readout over it stays legible.
 *
 * Code-drawn because react-native-svg's `FeTurbulence` renders nothing on native: a seeded tile
 * of grey cells repeated by an SVG pattern. The snow flickers by stepped jumps only when motion
 * is allowed; under reduced motion it holds still (present, not absent). Fills its parent and is
 * inert: hidden from assistive tech and never a touch target.
 */
const AnalogSnow = ({ reducedMotion, seed = 4 }: AnalogSnowProps) => {
  const theme = useTheme();
  const prefersReducedMotion = useReducedMotionPreference(reducedMotion);
  const step = useRef(new Animated.Value(0)).current;
  const cells = useMemo(() => {
    const random = seeded(seed);
    return Array.from({ length: TILE_CELLS * TILE_CELLS }, (_, i) => ({
      color: cellColor(random()),
      x: (i % TILE_CELLS) * CELL.width,
      y: Math.floor(i / TILE_CELLS) * CELL.height,
    }));
  }, [seed]);

  useEffect(() => {
    if (prefersReducedMotion !== false) {
      step.stopAnimation();
      step.setValue(0);
      return undefined;
    }
    const animation = Animated.loop(
      Animated.timing(step, {
        duration: FLICKER_STEP_MS * FLICKER_OFFSETS.length,
        easing: Easing.linear,
        toValue: FLICKER_OFFSETS.length,
        useNativeDriver: useNativeAnimationDriver,
      }),
    );
    animation.start();
    return () => animation.stop();
  }, [prefersReducedMotion, step]);

  // Stepped, not interpolated: each offset holds for its whole step, then jumps.
  const flicker = useMemo(() => {
    const inputRange = FLICKER_OFFSETS.flatMap((_, i) => [i, i + 0.999]);
    const along = (axis: 0 | 1) =>
      step.interpolate({ inputRange, outputRange: FLICKER_OFFSETS.flatMap((o) => [o[axis], o[axis]]) });
    return [{ translateX: along(0) }, { translateY: along(1) }];
  }, [step]);
  const patternId = `loomarr-snow-${seed}`;
  // The tile is ~600 native views. Built once per seed, so a re-render (a readout's motion
  // switching on at a key) never reconciles them again (#1781).
  const tile = useMemo(
    () => (
      <Svg height="100%" width="100%">
        <Defs>
          <Pattern height={TILE.height} id={patternId} patternUnits="userSpaceOnUse" width={TILE.width}>
            {cells.map((cell) => (
              <Rect
                fill={cell.color}
                height={CELL.height}
                key={`${cell.x}-${cell.y}`}
                width={CELL.width}
                x={cell.x}
                y={cell.y}
              />
            ))}
          </Pattern>
        </Defs>
        <Rect fill={`url(#${patternId})`} height="100%" width="100%" />
      </Svg>
    ),
    [cells, patternId],
  );

  return (
    <View
      aria-hidden
      pointerEvents="none"
      position="absolute"
      top={0}
      right={0}
      bottom={0}
      left={0}
      overflow="hidden"
    >
      <Animated.View
        style={{
          bottom: -OVERSIZE,
          left: -OVERSIZE,
          position: "absolute",
          right: -OVERSIZE,
          top: -OVERSIZE,
          transform: flicker,
        }}
      >
        {tile}
      </Animated.View>
      {/* The dark between the snow and the readout: the canvas at 90%, as on web. */}
      <View
        position="absolute"
        top={0}
        right={0}
        bottom={0}
        left={0}
        backgroundColor={theme.surfaceCanvas.val}
        opacity={0.9}
      />
      {/* Scanlines: one dark unit every third, holding still over the flickering snow. */}
      <Svg height="100%" style={{ position: "absolute" }} width="100%">
        <Defs>
          <Pattern height={3} id={`${patternId}-lines`} patternUnits="userSpaceOnUse" width={3}>
            <Rect fill="#000" fillOpacity={0.18} height={1} width={3} />
          </Pattern>
        </Defs>
        <Rect fill={`url(#${patternId}-lines)`} height="100%" width="100%" />
      </Svg>
    </View>
  );
};

export type { AnalogSnowProps };
export { AnalogSnow };
