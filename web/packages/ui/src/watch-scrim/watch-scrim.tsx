import { Layer, Scrim } from "@loomarr/design-system";
import { useState } from "react";
import type { LayoutChangeEvent } from "react-native";

// The watching screen's scrims (frontend-design §9.1, target). Each is one gradient as wide or as
// tall as the picture's edge, so the words that stand on it stand on no box.
const EDGES = {
  // Across the foot, strongest at the bottom edge and gone a little under half way up.
  foot: { feather: 0.7, from: "top", reach: "46%", strength: 0.95 },
  // Down the leading edge, as wide as the open list, fading out past it.
  side: { feather: 1 / 3, from: "right", reach: "60%", strength: 0.97 },
} as const;

/** The box a corner scrim stands behind: a control's place and size on the screen. */
interface ScrimBox {
  x: number;
  y: number;
  width: number;
  height: number;
}

/**
 * The top leading corner's ink, behind the control in `box`: full to the control's far edges,
 * then fading out past it, rightward over most of the control's own reach again and downward over
 * one and a half of its heights. The foot's ink, so its words read over a washed-out frame.
 */
const CornerScrim = ({ box }: { box: ScrimBox }) => {
  const right = box.x + box.width;
  const bottom = box.y + box.height;
  const feather = Math.round(right * 0.8);
  const soften = Math.round(box.height * 1.5);
  return (
    <Layer
      aria-hidden
      height={bottom}
      left={0}
      pointerEvents="none"
      testID="watch-scrim-corner"
      top={0}
      width={right + feather}
    >
      <Scrim feather={feather} from="right" soften={soften} strength={EDGES.foot.strength} />
    </Layer>
  );
};

/**
 * A scrim along one edge of its positioned parent, or in its top leading corner behind `box`. It
 * takes no pointer and is hidden from assistive technology. An edge's scrim draws once it knows
 * its size, so its fade is a share of it.
 */
const WatchScrim = ({ box, edge }: { box?: ScrimBox; edge: "corner" | "foot" | "side" }) => {
  const [length, setLength] = useState(0);
  if (edge === "corner") return box ? <CornerScrim box={box} /> : null;
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

export type { ScrimBox };
export { WatchScrim };
