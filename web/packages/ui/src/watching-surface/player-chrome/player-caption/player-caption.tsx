import { type Density, playerChrome, Text, ThemeScope } from "@loomarr/design-system";
import { View } from "react-native";

// The subtitles on the picture where the player draws them itself (#2127): a native player plays the
// sidecar's cues through no text track of its own. Drawn as a browser draws web Watch's text track —
// white lines on a dark box, centred at the picture's foot — so a programme reads the same on both.

interface PlayerCaptionProps {
  density: Density;
  /** The cue lines on screen now; nothing draws while there are none. */
  lines: readonly string[];
}

const PlayerCaption = ({ density, lines }: PlayerCaptionProps) =>
  lines.length > 0 ? (
    <ThemeScope theme="dark">
      <View
        accessibilityLiveRegion="polite"
        pointerEvents="none"
        style={{ alignItems: "center", bottom: 12, left: 12, position: "absolute", right: 12 }}
      >
        <View style={{ backgroundColor: playerChrome.scrimFoot, borderRadius: 4, paddingHorizontal: 8 }}>
          {lines.map((line, i) => (
            // Cue lines have no identity of their own; their place is their key.
            // biome-ignore lint/suspicious/noArrayIndexKey: lines are positional within one cue.
            <Text density={density} key={i} style={{ textAlign: "center" }} textRole="body" tone="primary">
              {line}
            </Text>
          ))}
        </View>
      </View>
    </ThemeScope>
  ) : null;

export type { PlayerCaptionProps };
export { PlayerCaption };
