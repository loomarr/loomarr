import { type Density, Icon, icons, playerChrome, Text, useFocusLook } from "@loomarr/design-system";
import { useState } from "react";
import { Pressable, View } from "react-native";

import { webOnly } from "../flat";

// The option list the Subtitles, Audio and Picture sheets share (§9.5): plain words, the selected
// option white with a check, an optional second line in a quieter voice. A choice is a radio; a list
// the viewer cannot change (the channel's audio language, set by an admin) is read-only.

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
  const row = {
    alignItems: "center",
    backgroundColor: option.selected || hovered ? playerChrome.surfaceRaised : "transparent",
    borderRadius: 6,
    flexDirection: "row",
    gap: 16,
    minHeight: 56,
    paddingHorizontal: 16,
    paddingVertical: 8,
    ...webOnly({
      outlineColor: playerChrome.signal,
      outlineStyle: focused ? "solid" : "none",
      outlineWidth: 2,
      transition: "background-color 120ms ease",
    }),
  } as const;
  const body = (
    <>
      <View style={{ flex: 1, gap: 4 }}>
        <Text density={density} strong={option.selected} textRole="body" tone="primary">
          {option.label}
        </Text>
        {option.detail ? (
          <Text density={density} textRole="compact" tone="muted">
            {option.detail}
          </Text>
        ) : null}
      </View>
      {option.selected ? (
        <Icon color={playerChrome.text} decorative glyph={icons.success} size="default" />
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
  <View aria-label={label} role={onSelect ? "radiogroup" : "list"} style={{ gap: 8 }}>
    {options.map((option) => (
      <OptionRow density={density} key={option.value} onSelect={onSelect} option={option} />
    ))}
  </View>
);

export type { PlayerOption, PlayerOptionsProps };
export { PlayerOptions };
