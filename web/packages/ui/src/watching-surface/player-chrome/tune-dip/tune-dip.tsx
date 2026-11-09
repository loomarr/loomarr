import { playerChrome, useReducedMotionPreference } from "@loomarr/design-system";
import { useEffect, useRef } from "react";
import { Animated } from "react-native";

// The warm flip's dip (§9.4): the picture goes dark once and comes back, in 330 ms, whenever the
// channel changes. Quiet on purpose: the maintainer found a full animation on every flip annoying, so
// this and the band's bar sweep are the whole of it. It never adds waiting time (it dresses the
// switch already under way) and under reduced motion there is no dip: the picture just changes.

const DIP_MS = 330;
const DEPTH = 0.7;

const TuneDip = ({ tuneKey }: { tuneKey: string }) => {
  const reduced = useReducedMotionPreference();
  const level = useRef(new Animated.Value(0)).current;
  const first = useRef(true);
  // biome-ignore lint/correctness/useExhaustiveDependencies: the key is the trigger
  useEffect(() => {
    if (first.current) {
      first.current = false;
      return;
    }
    if (reduced) return;
    level.setValue(DEPTH);
    Animated.timing(level, { duration: DIP_MS, toValue: 0, useNativeDriver: false }).start();
  }, [tuneKey]);
  return (
    <Animated.View
      pointerEvents="none"
      style={{
        backgroundColor: playerChrome.surface,
        bottom: 0,
        left: 0,
        opacity: level,
        position: "absolute",
        right: 0,
        top: 0,
        zIndex: 20,
      }}
    />
  );
};

export { TuneDip };
