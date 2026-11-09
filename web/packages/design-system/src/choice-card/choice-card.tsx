import { useTheme, View } from "@tamagui/core";
import type {
  ComponentProps,
  ComponentRef,
  ForwardRefExoticComponent,
  ReactNode,
  RefAttributes,
} from "react";
import { forwardRef, useState } from "react";
import { Pressable } from "react-native";

import { useFocusLook } from "../focus-look";
import { Text } from "../primitives";
import { semanticRadius, semanticSpace } from "../tokens";

type ChoiceCardAccent = "info" | "suggest";

type ChoiceCardProps = Omit<ComponentProps<typeof Pressable>, "children" | "style"> & {
  /**
   * The colour of the chosen and hovered border: `suggest` for a starting point the AI will build
   * on, `info` for a fact the library offers.
   */
  accent?: ChoiceCardAccent;
  /** One line under the title. */
  description?: string;
  /** A glyph or a spinner before the words. */
  leading?: ReactNode;
  /** This card is the chosen one of its set; it announces `aria-pressed` for both states. */
  selected?: boolean;
  title: string;
};

type ChoiceCardRef = ComponentRef<typeof Pressable>;

// A 15% wash of a #RRGGBB colour, the same tint a selected badge takes.
const wash = (hex: string, alpha: number) => {
  const n = Number.parseInt(hex.slice(1, 7), 16);
  return `rgba(${(n >> 16) & 255},${(n >> 8) & 255},${n & 255},${alpha})`;
};

/**
 * A choice drawn as a card: a raised surface with a hairline border, an optional leading glyph, a
 * title and a line of description. Hover and the chosen state take the accent's border, the chosen
 * one also a tint of it; focus is the one amber border. A set of cards is a list of buttons the
 * reader taps to pick one (a starter template, a facet of the library).
 */
const ChoiceCard: ForwardRefExoticComponent<ChoiceCardProps & RefAttributes<ChoiceCardRef>> = forwardRef<
  ChoiceCardRef,
  ChoiceCardProps
>(
  (
    {
      accent = "suggest",
      accessibilityState,
      description,
      disabled = false,
      leading,
      onBlur,
      onFocus,
      selected,
      title,
      ...props
    },
    ref,
  ) => {
    const theme = useTheme();
    const focusLook = useFocusLook({ onBlur, onFocus });
    const [hovered, setHovered] = useState(false);
    const toggle = selected !== undefined;
    const on = selected === true;
    const accentColor = accent === "info" ? theme.stateInfo.val : theme.accentSuggest.val;
    return (
      <Pressable
        accessibilityRole="button"
        accessibilityState={{ ...accessibilityState, disabled, selected: on }}
        aria-pressed={toggle ? on : undefined}
        disabled={disabled}
        onBlur={focusLook.onBlur}
        onFocus={focusLook.onFocus}
        onHoverIn={() => setHovered(true)}
        onHoverOut={() => setHovered(false)}
        ref={ref}
        style={({ pressed }) => ({ opacity: disabled ? 0.5 : pressed ? 0.82 : 1, width: "100%" })}
        {...props}
      >
        <View
          alignItems="flex-start"
          backgroundColor={on ? wash(accentColor, 0.15) : "$surfaceRaised"}
          borderColor={
            focusLook.focused
              ? "$actionFocus"
              : on || (hovered && !disabled)
                ? accentColor
                : "$borderDecorative"
          }
          borderRadius={semanticRadius.control}
          borderWidth={1}
          flexDirection="row"
          gap={semanticSpace.control}
          paddingHorizontal={semanticSpace.control}
          paddingVertical={semanticSpace.inline + 2}
        >
          {leading ? <View paddingTop={2}>{leading}</View> : null}
          <View flex={1} gap={2} minWidth={0}>
            <Text textRole="label" tone="primary">
              {title}
            </Text>
            {description ? (
              <Text textRole="caption" tone="muted">
                {description}
              </Text>
            ) : null}
          </View>
        </View>
      </Pressable>
    );
  },
);

ChoiceCard.displayName = "ChoiceCard";

export type { ChoiceCardAccent, ChoiceCardProps };
export { ChoiceCard };
