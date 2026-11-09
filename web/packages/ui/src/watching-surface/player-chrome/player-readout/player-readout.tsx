import { type Density, playerChrome, Text } from "@loomarr/design-system";
import { View } from "react-native";

// The channel readout (§9.2): the number being typed, on a solid ink backing so it reads over any
// frame, white (amber stays on the block and the play key). It is the one place the channel is named
// while the band is down: for digits being typed, and for a flip made with the controls hidden.

interface PlayerReadoutProps {
  density: Density;
  /** The digits so far, dashed out to the longest channel number. */
  digits: string;
  /** No channel has this number. */
  missed?: boolean;
  /** Distance from the frame's top: the chrome starts it below the top row and the watermark. */
  top: number;
}

const PlayerReadout = ({ density, digits, missed = false, top }: PlayerReadoutProps) => (
  <View
    role="status"
    style={{
      alignItems: "flex-end",
      backgroundColor: playerChrome.tooltip,
      gap: 4,
      paddingHorizontal: 24,
      paddingVertical: 16,
      position: "absolute",
      right: 48,
      top,
      zIndex: 40,
    }}
  >
    <Text density={density} strong textRole="display" tone="primary">
      {digits}
    </Text>
    <Text density={density} textRole="compact" tone={missed ? "live" : "secondary"}>
      {missed ? "No such channel" : "Enter channel"}
    </Text>
  </View>
);

export type { PlayerReadoutProps };
export { PlayerReadout };
