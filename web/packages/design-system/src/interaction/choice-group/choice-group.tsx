import { View } from "@tamagui/core";
import { Pressable } from "react-native";

import { useFocusLook } from "../../focus-look";
import { Surface } from "../../primitives";
import { Text } from "../../text";
import { type Density, semanticSpace, semanticTargets } from "../../tokens";
import { boxFocusRing } from "../box-focus-ring";
import type { ChoiceGroupProps, ChoiceOption } from "./choice-group.type";

const ChoiceRow = ({
  checked,
  density,
  onPress,
  option,
}: {
  checked: boolean;
  density: Density;
  onPress: () => void;
  option: ChoiceOption<string>;
}) => {
  const { focused, ...focusHandlers } = useFocusLook();
  return (
    <Pressable
      accessibilityLabel={option.label}
      accessibilityRole="radio"
      accessibilityState={{ checked, disabled: option.disabled }}
      aria-checked={checked}
      aria-disabled={option.disabled || undefined}
      disabled={option.disabled}
      {...focusHandlers}
      onPress={onPress}
      style={{
        alignItems: "center",
        flexDirection: "row",
        gap: semanticSpace.control,
        minHeight: semanticTargets[density],
        opacity: option.disabled ? 0.55 : 1,
        // The ring around the dot is the one focus look; the browser would add its own around the
        // whole row. Chrome's `auto` ring ignores a zero width, so the style is named too.
        outlineStyle: "solid",
        outlineWidth: 0,
      }}
    >
      <Surface
        alignItems="center"
        // A chosen option is neutral-pressed (#1909), like a checked box; amber is the focus ring.
        backgroundColor={checked ? "surfaceElevated" : "surfaceCanvas"}
        borderColor={checked ? "contentPrimary" : "borderControl"}
        borderRadius="round"
        height={density === "tv" ? 36 : 24}
        justifyContent="center"
        width={density === "tv" ? 36 : 24}
        {...boxFocusRing(focused, density)}
      >
        {checked ? (
          <View backgroundColor="contentPrimary" borderRadius="round" height="45%" width="45%" />
        ) : null}
      </Surface>
      <View flex={1} gap={2}>
        <Text density={density} textRole="label">
          {option.label}
        </Text>
        {option.description ? (
          <Text density={density} textRole="hint">
            {option.description}
          </Text>
        ) : null}
      </View>
    </Pressable>
  );
};

const ChoiceGroup = <Value extends string>({
  density = "pointer",
  label,
  onValueChange,
  options,
  value,
}: ChoiceGroupProps<Value>) => (
  <View aria-label={label} role="radiogroup" gap="inline">
    <Text density={density} textRole="label">
      {label}
    </Text>
    {options.map((option) => (
      <ChoiceRow
        checked={option.value === value}
        density={density}
        key={option.value}
        onPress={() => onValueChange(option.value)}
        option={option}
      />
    ))}
  </View>
);

export { ChoiceGroup };
