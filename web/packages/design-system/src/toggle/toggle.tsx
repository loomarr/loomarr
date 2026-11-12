import { View } from "@tamagui/core";
import { Pressable } from "react-native";

import { useFocusLook } from "../focus-look";
import { Icon } from "../icon";
import { icons } from "../icons";
import { boxFocusRing } from "../interaction/interaction";
import { Surface } from "../primitives";
import { Text } from "../text";
import { semanticSpace, semanticTargets } from "../tokens";
import type { ToggleProps } from "./toggle.type";

// The native adapter: one Pressable carrying the checkbox or switch role, so a phone, tablet or TV
// reaches the whole row as one target. The web adapter (toggle.web.tsx) draws the same box on Base
// UI so a browser gets a labelable control.
const Toggle = ({
  "aria-describedby": describedBy,
  checked,
  density = "pointer",
  description,
  disabled = false,
  id,
  kind = "checkbox",
  label,
  labelHidden = false,
  onCheckedChange,
}: ToggleProps) => {
  const switchWidth = density === "tv" ? 72 : density === "touch" ? 56 : 48;
  const controlSize = density === "tv" ? 36 : density === "touch" ? 28 : 24;
  const { focused, ...focusHandlers } = useFocusLook();
  return (
    <Pressable
      accessibilityLabel={label}
      accessibilityRole={kind}
      accessibilityState={{ checked, disabled }}
      aria-checked={checked}
      aria-describedby={describedBy}
      id={id}
      aria-disabled={disabled || undefined}
      disabled={disabled}
      {...focusHandlers}
      onPress={() => onCheckedChange(!checked)}
      style={{
        alignItems: "center",
        flexDirection: "row",
        gap: semanticSpace.control,
        minHeight: semanticTargets[density],
        opacity: disabled ? 0.55 : 1,
      }}
    >
      {/* Checked is a selection, so it is neutral-pressed (#1909): elevated fill with a foreground
          border and mark. Amber is the focus ring around the box, outside a gap. */}
      <Surface
        alignItems={kind === "switch" ? (checked ? "flex-end" : "flex-start") : "center"}
        backgroundColor={checked ? "surfaceElevated" : "surfaceCanvas"}
        borderColor={checked ? "contentPrimary" : "borderControl"}
        borderRadius={kind === "switch" ? "round" : "control"}
        height={controlSize}
        justifyContent="center"
        padding={kind === "switch" ? 3 : 0}
        width={kind === "switch" ? switchWidth : controlSize}
        {...boxFocusRing(focused, density)}
      >
        {kind === "switch" ? (
          <View
            backgroundColor={checked ? "contentPrimary" : "contentSecondary"}
            borderRadius="round"
            height={controlSize - 8}
            width={controlSize - 8}
          />
        ) : checked ? (
          <Icon
            decorative
            glyph={icons.success}
            size={density === "tv" ? "control" : "compact"}
            tone="content"
          />
        ) : null}
      </Surface>
      {labelHidden ? null : (
        <View flex={1} gap={2}>
          <Text density={density} textRole="label">
            {label}
          </Text>
          {description ? (
            <Text density={density} textRole="hint">
              {description}
            </Text>
          ) : null}
        </View>
      )}
    </Pressable>
  );
};

export { Toggle };
