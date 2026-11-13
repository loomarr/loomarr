import { type Density, playerChrome, Text } from "@loomarr/design-system";
import { View } from "react-native";

// A subtitle, where the player puts it (§9.5, variant J): centred on what shows of the picture, each
// line on its own near-black box. The chrome places it: lifted clear of the band while the controls
// are up, at the picture's foot while they are down. A television's line is short (about 32
// characters, as broadcast captions are), which also keeps it clear of a menu beside it.

/** How wide a line may run before it wraps, by where it is drawn. */
const CAPTION_WIDTH = { landscape: 320, pointer: 720, tv: 320, upright: 340 } as const;

interface PlayerCaptionProps {
  /** Distance from the frame's foot to the lowest line. */
  bottom: number;
  density: Density;
  lines: readonly string[];
  /** Room kept clear on the right (a side sheet), so the line is centred in what shows of the picture. */
  right?: number;
  where: keyof typeof CAPTION_WIDTH;
}

const PlayerCaption = ({ bottom, density, lines, right = 0, where }: PlayerCaptionProps) =>
  lines.length === 0 ? null : (
    <View
      // The lines are what is said on screen; a reader has the programme's own audio.
      aria-hidden
      pointerEvents="none"
      style={{ alignItems: "center", bottom, gap: 2, left: 0, position: "absolute", right, zIndex: 35 }}
      testID="player-caption"
    >
      {lines.map((line, i) => (
        <View
          // biome-ignore lint/suspicious/noArrayIndexKey: a cue's lines are positional and may repeat.
          key={i}
          style={{
            backgroundColor: playerChrome.caption,
            borderRadius: 2,
            maxWidth: CAPTION_WIDTH[where],
            paddingHorizontal: 10,
            paddingVertical: 2,
          }}
        >
          <Text
            density={density}
            style={{ textAlign: "center" }}
            textRole={where === "upright" ? "subtitleUpright" : "subtitle"}
          >
            {line}
          </Text>
        </View>
      ))}
    </View>
  );

export type { PlayerCaptionProps };
export { PlayerCaption };
