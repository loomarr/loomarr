import { type Density, Icon, icons, playerChrome, Text, Toggle } from "@loomarr/design-system";
import { useState } from "react";
import { Pressable, View } from "react-native";

// The Settings sheet's body (§9, mock "settings"). Quality first: Auto (`Auto · 1080p`, naming the
// rung on air) and the channel's own rungs, the viewer's to choose (#2088 decision 7). Then the
// channel's audio language, shown — it is the channel's default, set by an admin on Channel info,
// until a second rendition lets each viewer choose (#2088 step 9). Subtitles, when the programme
// carries any: Off and each track, the viewer's to choose (#2088 decision 5). Then the channel-change
// sound, and last, the way on to Playback info.

interface SettingsOption {
  label: string;
  selected: boolean;
  value: string;
}

interface PlayerSettingsProps {
  /** The channel's audio languages, the one airing marked; read-only here. */
  audio: readonly SettingsOption[];
  density: Density;
  /** Opens Playback info (#2088 decision 13), the sheet's last row. */
  onPlaybackInfo?: () => void;
  onSwitchSound: (on: boolean) => void;
  /** Auto and the rungs; omitted while the player has no ladder to offer (native HLS picks itself). */
  quality?: { onSelect: (value: string) => void; options: readonly SettingsOption[] };
  /** Off and the programme's subtitle tracks; omitted where it carries none. */
  subtitles?: { onSelect: (value: string) => void; options: readonly SettingsOption[] };
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
  <Text density={density} shout textRole="guideMeta" tone="secondary" tracking={0.16}>
    {label}
  </Text>
);

// The way on to another sheet: icon, label, and a chevron, behind a rule.
const NavRow = ({ density, label, onPress }: { density: Density; label: string; onPress: () => void }) => {
  const [hovered, setHovered] = useState(false);
  return (
    <View style={{ borderTopColor: playerChrome.glassRing, borderTopWidth: 1, paddingTop: 8 }}>
      <Pressable
        aria-label={label}
        onHoverIn={() => setHovered(true)}
        onHoverOut={() => setHovered(false)}
        onPress={onPress}
        role="button"
        style={{
          alignItems: "center",
          backgroundColor: hovered ? playerChrome.glassTop : "transparent",
          borderRadius: 6,
          flexDirection: "row",
          gap: 12,
          paddingHorizontal: 12,
          paddingVertical: 10,
        }}
      >
        <Icon color={playerChrome.textSecondary} decorative glyph={icons.info} size="compact" />
        <Text density={density} style={{ flex: 1, textAlign: "left" }} textRole="compact" tone="primary">
          {label}
        </Text>
        <Icon color={playerChrome.textMuted} decorative glyph={icons.disclosure} size="compact" />
      </Pressable>
    </View>
  );
};

const PlayerSettings = ({
  audio,
  density,
  onPlaybackInfo,
  onSwitchSound,
  quality,
  subtitles,
  switchSound,
}: PlayerSettingsProps) => (
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
    {subtitles ? (
      <View style={{ gap: 6 }}>
        <SectionLabel density={density} label="Subtitles" />
        <View aria-label="Subtitles" role="radiogroup">
          {subtitles.options.map((option) => (
            <OptionRow density={density} key={option.value} onSelect={subtitles.onSelect} option={option} />
          ))}
        </View>
      </View>
    ) : null}
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
    {onPlaybackInfo ? <NavRow density={density} label="Playback info" onPress={onPlaybackInfo} /> : null}
  </View>
);

export type { PlayerSettingsProps };
export { PlayerSettings };
