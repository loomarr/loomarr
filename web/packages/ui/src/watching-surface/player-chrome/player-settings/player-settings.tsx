import { type Density, Icon, icons, playerChrome, Text, Toggle } from "@loomarr/design-system";
import { useState } from "react";
import { Pressable, View } from "react-native";

// The Settings sheet's body (§9, mock "settings"). Quality first: Auto (`Auto · 1080p`, naming the
// rung on air) and the channel's own rungs, the viewer's to choose (#2088 decision 7). Then the
// channel's audio language, shown — it is the channel's default, set by an admin on Channel info,
// until a second rendition lets each viewer choose (#2088 step 9) — and the channel-change sound.

interface SettingsOption {
  label: string;
  selected: boolean;
  value: string;
}

interface PlayerSettingsProps {
  /** The channel's audio languages, the one airing marked; read-only here. */
  audio: readonly SettingsOption[];
  density: Density;
  onSwitchSound: (on: boolean) => void;
  /** Auto and the rungs; omitted while the player has no ladder to offer (native HLS picks itself). */
  quality?: { onSelect: (value: string) => void; options: readonly SettingsOption[] };
  switchSound: boolean;
}

// One row: a check beside the selected option. A choice is a radio; a read-only list item otherwise.
const OptionRow = ({
  density,
  onSelect,
  option,
}: {
  density: Density;
  onSelect?: (value: string) => void;
  option: SettingsOption;
}) => {
  const [hovered, setHovered] = useState(false);
  const body = (
    <>
      <View style={{ alignItems: "center", width: 16 }}>
        {option.selected ? (
          <Icon color={playerChrome.signal} decorative glyph={icons.success} size="compact" />
        ) : null}
      </View>
      <Text density={density} textRole="compact" tone={option.selected ? "signal" : "primary"}>
        {option.label}
      </Text>
    </>
  );
  const row = {
    alignItems: "center",
    borderRadius: 6,
    flexDirection: "row",
    gap: 12,
    paddingHorizontal: 12,
    paddingVertical: 8,
  } as const;
  return onSelect ? (
    <Pressable
      // aria-* directly: RN-web drops accessibilityState.checked, which would leave the choice unread.
      aria-checked={option.selected}
      aria-label={option.label}
      role="radio"
      onHoverIn={() => setHovered(true)}
      onHoverOut={() => setHovered(false)}
      onPress={() => onSelect(option.value)}
      style={{ ...row, backgroundColor: hovered ? playerChrome.glassTop : "transparent" }}
    >
      {body}
    </Pressable>
  ) : (
    <View aria-current={option.selected || undefined} role="listitem" style={row}>
      {body}
    </View>
  );
};

const SectionLabel = ({ density, label }: { density: Density; label: string }) => (
  <Text density={density} shout textRole="mono" tone="secondary" tracking={0.16}>
    {label}
  </Text>
);

const PlayerSettings = ({ audio, density, onSwitchSound, quality, switchSound }: PlayerSettingsProps) => (
  <View style={{ gap: 20 }}>
    {quality ? (
      <View style={{ gap: 6 }}>
        <SectionLabel density={density} label="Quality" />
        <View aria-label="Quality" role="radiogroup">
          {quality.options.map((option) => (
            <OptionRow density={density} key={option.value} onSelect={quality.onSelect} option={option} />
          ))}
        </View>
      </View>
    ) : null}
    <View style={{ gap: 6 }}>
      <SectionLabel density={density} label="Audio" />
      <View aria-label="Audio language" role="list">
        {audio.map((option) => (
          <OptionRow density={density} key={option.value} option={option} />
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
