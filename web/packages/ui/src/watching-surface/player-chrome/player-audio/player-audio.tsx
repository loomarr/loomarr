import { type Density, playerChrome, Text, Toggle } from "@loomarr/design-system";
import { View } from "react-native";

import { type PlayerOption, PlayerOptions } from "../player-options";

// Audio's body (§9.5): the channel's audio languages, the one airing marked and read-only (an admin
// sets it on the channel's page, until a second rendition lets each viewer choose), the line that
// says so, then the channel-change sound under a hairline.

interface PlayerAudioProps {
  density: Density;
  onSwitchSound: (on: boolean) => void;
  options: readonly PlayerOption[];
  switchSound: boolean;
}

const PlayerAudio = ({ density, onSwitchSound, options, switchSound }: PlayerAudioProps) => {
  const inset = density === "pointer" ? 10 : 12;
  return (
    <View style={{ gap: 4 }}>
      <PlayerOptions density={density} label="Audio language" options={options} />
      <View style={{ paddingBottom: 8, paddingHorizontal: inset }}>
        <Text density={density} textRole="cardMeta" tone="muted">
          The channel’s language. An admin sets it on the channel’s page.
        </Text>
      </View>
      <View
        style={{
          borderTopColor: playerChrome.hairline,
          borderTopWidth: 1,
          paddingHorizontal: inset,
          paddingVertical: 10,
        }}
      >
        <Toggle
          checked={switchSound}
          density={density}
          kind="switch"
          label="Channel-change sound"
          onCheckedChange={onSwitchSound}
        />
      </View>
    </View>
  );
};

export type { PlayerAudioProps };
export { PlayerAudio };
