import type { ClientDestination } from "@loomarr/ui";
import type { ReactNode } from "react";
import { View } from "react-native";

interface TvScreensProps {
  active: ClientDestination;
  /** The Guide or Surf, drawn over Watching only while one of them is showing. */
  overlay: ReactNode;
  /** The Watching surface with its native player: always mounted. */
  watching: ReactNode;
}

/**
 * Watching stays mounted beneath the Guide and Surf, so its native player keeps its stream and its
 * place instead of restarting when the viewer comes back; the other destinations draw over it.
 */
const TvScreens = ({ active, overlay, watching }: TvScreensProps) => (
  <View style={{ flex: 1 }}>
    {watching}
    {active === "watching" ? null : (
      <View style={{ bottom: 0, left: 0, position: "absolute", right: 0, top: 0 }}>{overlay}</View>
    )}
  </View>
);

export type { TvScreensProps };
export { TvScreens };
