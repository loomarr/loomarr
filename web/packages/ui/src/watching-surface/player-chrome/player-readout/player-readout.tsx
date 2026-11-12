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
  /** The channel the digits reach, named under them; "Enter channel" until they reach one. */
  name?: string;
  /**
   * When the typed number tunes by itself ("auto-tunes in 1.2 s"): a remote's digits tune after a
   * pause the viewer can see and lengthen (#1659 N4, WCAG 2.2.1).
   */
  countdown?: string;
  /** Distance from the frame's top: the chrome starts it below the top row and the watermark. */
  top: number;
  /** Distance from the frame's right edge: the chrome's side margin (an upright phone's is narrower). */
  right?: number;
}

const PlayerReadout = ({
  countdown,
  density,
  digits,
  missed = false,
  name,
  right = 48,
  top,
}: PlayerReadoutProps) => (
  <View
    role="status"
    style={{
      alignItems: "flex-end",
      backgroundColor: playerChrome.tooltip,
      gap: 4,
      paddingHorizontal: 24,
      paddingVertical: 16,
      position: "absolute",
      right,
      top,
      zIndex: 40,
    }}
  >
    <Text density={density} strong textRole="display" tone="primary">
      {digits}
    </Text>
    <Text density={density} numberOfLines={1} textRole="compact" tone={missed ? "live" : "secondary"}>
      {missed ? "No such channel" : (name ?? "Enter channel")}
    </Text>
    {countdown && !missed ? (
      <Text density={density} textRole="compact" tone="muted">
        {countdown}
      </Text>
    ) : null}
  </View>
);

export type { PlayerReadoutProps };
export { PlayerReadout };
