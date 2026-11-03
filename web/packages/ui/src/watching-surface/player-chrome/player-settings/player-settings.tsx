import { type Density, Icon, icons, playerChrome, Text, Toggle } from "@loomarr/design-system";
import { View } from "react-native";

// The Settings sheet's body (§9, mock "settings"). What the player can change today: the channel's
// audio language is shown — it is the channel's default, set by an admin on Channel info, until a
// second rendition lets each viewer choose (#2088 step 9) — and the channel-change sound. Quality
// rungs join it in step 4.

interface PlayerSettingsProps {
  /** The channel's audio languages, the one airing marked; read-only here. */
  audio: readonly { label: string; selected: boolean; value: string }[];
  density: Density;
  onSwitchSound: (on: boolean) => void;
  switchSound: boolean;
}

const SectionLabel = ({ density, label }: { density: Density; label: string }) => (
  <Text density={density} shout textRole="guideMeta" tone="secondary" tracking={0.16}>
    {label}
  </Text>
);

const PlayerSettings = ({ audio, density, onSwitchSound, switchSound }: PlayerSettingsProps) => (
  <View style={{ gap: 20 }}>
    <View style={{ gap: 6 }}>
      <SectionLabel density={density} label="Audio" />
      <View aria-label="Audio language" role="list">
        {audio.map((option) => (
          <View
            aria-current={option.selected || undefined}
            key={option.value}
            role="listitem"
            style={{
              alignItems: "center",
              flexDirection: "row",
              gap: 12,
              paddingHorizontal: 12,
              paddingVertical: 8,
            }}
          >
            <View style={{ alignItems: "center", width: 16 }}>
              {option.selected ? (
                <Icon color={playerChrome.signal} decorative glyph={icons.success} size="compact" />
              ) : null}
            </View>
            <Text density={density} textRole="compact" tone={option.selected ? "signal" : "primary"}>
              {option.label}
            </Text>
          </View>
        ))}
      </View>
      <Text density={density} textRole="hint" tone="muted">
        The channel’s language. An admin sets it on the channel’s page.
      </Text>
    </View>
    <View style={{ gap: 6 }}>
      <SectionLabel density={density} label="Sound" />
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

export type { PlayerSettingsProps };
export { PlayerSettings };
