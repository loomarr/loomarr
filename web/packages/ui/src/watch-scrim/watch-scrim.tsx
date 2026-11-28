import { Layer, Scrim } from "@loomarr/design-system";
import { useState } from "react";
import type { LayoutChangeEvent } from "react-native";

// The watching screen's two scrims (frontend-design §9.1, target). Each is one gradient as wide or
// as tall as the picture's edge, so the words that stand on it stand on no box.
const EDGES = {
  // Across the foot, strongest at the bottom edge and gone a little under half way up.
  foot: { feather: 0.7, from: "top", reach: "46%", strength: 0.95 },
  // Down the leading edge, as wide as the open list, fading out past it.
  side: { feather: 1 / 3, from: "right", reach: "60%", strength: 0.97 },
} as const;

/**
 * A scrim along one edge of its positioned parent. It takes no pointer and is hidden from assistive
 * technology. It draws once it knows its size, so its fade is a share of it.
 */
const WatchScrim = ({ edge }: { edge: "foot" | "side" }) => {
  const [length, setLength] = useState(0);
  const { feather, from, reach, strength } = EDGES[edge];
  const foot = edge === "foot";
  const onLayout = (event: LayoutChangeEvent) => {
    const { height, width } = event.nativeEvent.layout;
    setLength(foot ? height : width);
  };
  return (
    <Layer
      aria-hidden
      bottom={0}
      left={0}
      onLayout={onLayout}
      pointerEvents="none"
      testID={`watch-scrim-${edge}`}
      {...(foot ? { height: reach, right: 0 } : { top: 0, width: reach })}
    >
      {length ? <Scrim feather={Math.round(length * feather)} from={from} strength={strength} /> : null}
    </Layer>
  );
};

export { WatchScrim };
