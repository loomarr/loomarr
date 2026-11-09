import { type Density, Text, Toggle } from "@loomarr/design-system";
import { View } from "react-native";

import { type PlayerOption, PlayerOptions } from "../player-options";

// The Audio sheet's body (§9.5): the channel's audio languages, the one airing marked and read-only
// (an admin sets it on the channel's page, until a second rendition lets each viewer choose), then
// the channel-change sound.

interface PlayerAudioProps {
  density: Density;
  onSwitchSound: (on: boolean) => void;
  options: readonly PlayerOption[];
  switchSound: boolean;
}

const PlayerAudio = ({ density, onSwitchSound, options, switchSound }: PlayerAudioProps) => (
  <View style={{ gap: 24 }}>
    <View style={{ gap: 8 }}>
      <PlayerOptions density={density} label="Audio language" options={options} />
      <View style={{ paddingHorizontal: 16 }}>
        <Text density={density} textRole="hint" tone="muted">
          The channel’s language. An admin sets it on the channel’s page.
        </Text>
      </View>
    </View>
    <Toggle
      checked={switchSound}
      density={density}
      kind="switch"
      label="Channel-change sound"
      onCheckedChange={onSwitchSound}
    />
  </View>
);

export type { PlayerAudioProps };
export { PlayerAudio };
