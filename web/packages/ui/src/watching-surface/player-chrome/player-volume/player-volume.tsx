import { Icon, icons, playerChrome, Text } from "@loomarr/design-system";
import { View } from "react-native";

import { VolumeSlider } from "../player-transport";

// The volume level (§9.5, variant J, desktop): it opens in the menu's place, from the Volume key, and
// shows for a moment whenever the level changes (M to mute, the slider itself). The glyph says
// whether the sound is on; the number is the level, "Off" when muted. Dragging to zero mutes.

interface PlayerVolumeProps {
  muted: boolean;
  /** 0–1. Zero mutes, and anything above it un-mutes, as every player does. */
  onVolume: (volume: number) => void;
  /** 0–1. */
  volume: number;
}

const PlayerVolume = ({ muted, onVolume, volume }: PlayerVolumeProps) => (
  <View
    style={{
      alignItems: "center",
      backgroundColor: playerChrome.band,
      borderColor: playerChrome.hairline,
      borderRadius: 12,
      borderWidth: 1,
      flexDirection: "row",
      gap: 12,
      height: 48,
      maxWidth: "100%",
      paddingHorizontal: 16,
      width: 280,
    }}
    testID="player-volume"
  >
    <Icon
      color={playerChrome.text}
      decorative
      glyph={muted ? icons.volumeMuted : icons.volume}
      size="default"
    />
    <VolumeSlider density="pointer" fill muted={muted} onVolume={onVolume} volume={volume} />
    <Text density="pointer" style={{ minWidth: 24, textAlign: "right" }} textRole="mono" tone="secondary">
      {muted ? "Off" : String(Math.round(volume * 100))}
    </Text>
  </View>
);

export type { PlayerVolumeProps };
export { PlayerVolume };
