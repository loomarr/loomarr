import { View } from "@tamagui/core";
import type { ComponentProps, ComponentRef, ForwardRefExoticComponent, RefAttributes } from "react";
import { forwardRef } from "react";
import { Pressable } from "react-native";

import { useFocusLook } from "../focus-look";
import { Text, type TextTone } from "../text";
import { semanticRadius, semanticSpace } from "../tokens";

type ChipTone = Extract<TextTone, "danger" | "info" | "muted" | "success" | "warning">;

type ChipProps = Omit<ComponentProps<typeof Pressable>, "children" | "style"> & {
  /** The label. It names the chip to a screen reader unless `accessibilityLabel` says more. */
  children: string;
  /**
   * The chip is on. A toggle chip announces `aria-pressed` for both states, so an off chip reads as
   * a toggle rather than a plain button; a chip that only runs an action leaves this out.
   */
  selected?: boolean;
  /** Mono for a short code or a count ("2H", "AS AIRED"); the body face otherwise. */
  mono?: boolean;
  /** `pill` rounds the ends fully, for a suggestion the reader taps to fill a field. */
  shape?: "box" | "pill";
  /**
   * The label colour: a status the chip filters by (danger, warning, success) or a link-like info.
   * A muted chip takes the body colour while it is on.
   */
  tone?: ChipTone;
};

type ChipRef = ComponentRef<typeof Pressable>;

/**
 * A compact toggle or choice chip: a hairline outline round a short label. On is neutral, an
 * elevated fill with a foreground border and a heavier label, as a selected Action is (amber is
 * focus, never selection); off is the control border with a muted label. Focus is the one amber
 * border.
 */
// Annotated so the exported declaration names Pressable's instance type rather than React Native's
// internal element class.
const Chip: ForwardRefExoticComponent<ChipProps & RefAttributes<ChipRef>> = forwardRef<ChipRef, ChipProps>(
  (
    {
      accessibilityState,
      children,
      disabled = false,
      mono = false,
      onBlur,
      onFocus,
      selected,
      shape = "box",
      tone = "muted",
      ...props
    },
    ref,
  ) => {
    const focusLook = useFocusLook({ onBlur, onFocus });
    const toggle = selected !== undefined;
    const on = selected === true;
    return (
      <Pressable
        {...props}
        accessibilityRole="button"
        accessibilityState={{ ...accessibilityState, disabled, selected: on }}
        aria-pressed={toggle ? on : undefined}
        disabled={disabled}
        onBlur={focusLook.onBlur}
        onFocus={focusLook.onFocus}
        ref={ref}
        style={({ pressed }) => ({ opacity: disabled ? 0.55 : pressed ? 0.82 : 1 })}
      >
        <View
          alignItems="center"
          backgroundColor={on ? "surfaceElevated" : "transparent"}
          borderColor={focusLook.focused ? "actionFocus" : on ? "contentPrimary" : "borderControl"}
          borderRadius={shape === "pill" ? semanticRadius.round : semanticSpace.inline}
          borderWidth={1}
          justifyContent="center"
          minWidth={semanticSpace.section}
          paddingHorizontal={semanticSpace.inline + 2}
          paddingVertical={semanticSpace.inline / 2}
        >
          <Text
            strong={on}
            textRole={mono ? "mono" : "caption"}
            tone={on && tone === "muted" ? "primary" : tone}
          >
            {children}
          </Text>
        </View>
      </Pressable>
    );
  },
);

Chip.displayName = "Chip";

export type { ChipProps, ChipTone };
export { Chip };
