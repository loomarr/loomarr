import { type Density, Icon, icons, playerChrome, Text, useFocusLook } from "@loomarr/design-system";
import { useState } from "react";
import { Pressable, View } from "react-native";

import { playerRow, rowRole } from "../player-menu";

// The option list Subtitles, Audio and Picture share (§9.5, variant J), in a menu or a sheet alike:
// plain words, the one in use strong with a check, an optional second line in a quieter voice. A
// choice is a radio; a list the viewer cannot change (the channel's audio language, set by an admin)
// is read-only, its others set quieter.

interface PlayerOption {
  /** A quieter second line ("Best for your connection"). */
  detail?: string;
  label: string;
  selected: boolean;
  value: string;
}

interface PlayerOptionsProps {
  density: Density;
  /** The group's name, read by assistive technology. */
  label: string;
  onSelect?: (value: string) => void;
  options: readonly PlayerOption[];
}

const OptionRow = ({
  density,
  onSelect,
  option,
}: {
  density: Density;
  onSelect?: (value: string) => void;
  option: PlayerOption;
}) => {
  const [hovered, setHovered] = useState(false);
  const { focused, ...focusHandlers } = useFocusLook();
  const row = playerRow(density, { focused, hovered: hovered && onSelect !== undefined });
  const body = (
    <>
      <View style={{ flex: 1, gap: 2, minWidth: 0 }}>
        <Text
          density={density}
          strong={option.selected}
          textRole={rowRole(density)}
          tone={onSelect || option.selected ? "primary" : "secondary"}
        >
          {option.label}
        </Text>
        {option.detail ? (
          <Text density={density} textRole="cardMeta" tone="muted">
            {option.detail}
          </Text>
        ) : null}
      </View>
      {option.selected ? (
        <Icon
          color={playerChrome.text}
          decorative
          glyph={icons.success}
          size={density === "pointer" ? "compact" : "default"}
        />
      ) : null}
    </>
  );
  return onSelect ? (
    <Pressable
      // aria-* directly: RN-web drops accessibilityState.checked, which would leave the choice unread.
      aria-checked={option.selected}
      aria-label={option.label}
      role="radio"
      {...focusHandlers}
      onHoverIn={() => setHovered(true)}
      onHoverOut={() => setHovered(false)}
      onPress={() => onSelect(option.value)}
      style={row}
    >
      {body}
    </Pressable>
  ) : (
    <View aria-current={option.selected || undefined} role="listitem" style={row}>
      {body}
    </View>
  );
};

const PlayerOptions = ({ density, label, onSelect, options }: PlayerOptionsProps) => (
  <View aria-label={label} role={onSelect ? "radiogroup" : "list"} style={{ gap: 2 }}>
    {options.map((option) => (
      <OptionRow density={density} key={option.value} onSelect={onSelect} option={option} />
    ))}
  </View>
);

export type { PlayerOption, PlayerOptionsProps };
export { PlayerOptions };
