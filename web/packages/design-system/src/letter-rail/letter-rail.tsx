import { View } from "@tamagui/core";
import { useState } from "react";
import { View as NativeView, Pressable } from "react-native";

import { useFocusLook } from "../focus-look";
import { TamaguiText } from "../text-base";

// "#" stands for every title that starts with a digit or any other character. It leads, as such
// titles sort first.
const LETTERS = ["#", ..."ABCDEFGHIJKLMNOPQRSTUVWXYZ"];

// The 24 px minimum target (WCAG 2.5.8): 27 of them fit a desktop window.
const SIZE = 24;

type LetterRailProps = {
  /** Names the rail to a screen reader. */
  label?: string;
  /** The letters that have something to jump to; every other letter shows disabled. */
  available: readonly string[];
  /** The letter the reader is at, drawn raised. */
  current?: string;
  onSelect: (letter: string) => void;
};

const Letter = ({
  letter,
  disabled,
  current,
  onSelect,
}: {
  letter: string;
  disabled: boolean;
  current: boolean;
  onSelect: (letter: string) => void;
}) => {
  const focus = useFocusLook();
  const [hovered, setHovered] = useState(false);
  return (
    <Pressable
      accessibilityLabel={
        letter === "#" ? "Jump to numbers and symbols, #" : `Jump to titles starting with ${letter}`
      }
      accessibilityRole="button"
      accessibilityState={{ disabled }}
      aria-current={current ? "true" : undefined}
      disabled={disabled}
      onBlur={focus.onBlur}
      onFocus={focus.onFocus}
      onHoverIn={() => setHovered(true)}
      onHoverOut={() => setHovered(false)}
      onPress={() => onSelect(letter)}
    >
      <View
        alignItems="center"
        backgroundColor={current || (hovered && !disabled) ? "surfaceRaised" : "transparent"}
        borderColor={focus.focused ? "actionFocus" : "transparent"}
        borderRadius="control"
        borderWidth={2}
        height={SIZE}
        justifyContent="center"
        opacity={disabled ? 0.38 : 1}
        width={SIZE}
      >
        <TamaguiText color={current ? "contentPrimary" : "contentSecondary"} fontSize={12}>
          {letter}
        </TamaguiText>
      </View>
    </Pressable>
  );
};

/**
 * The letters down the side of a long alphabetical list, "#" then A to Z, each a button that jumps
 * to its first entry. Every letter is drawn, so the rail keeps its shape; one with nothing to jump
 * to is disabled and out of the tab order. Pointer and keyboard: it is a desktop control.
 */
const LetterRail = ({ label = "Jump to a letter", available, current, onSelect }: LetterRailProps) => (
  <NativeView aria-label={label} role="navigation">
    {LETTERS.map((letter) => (
      <Letter
        current={letter === current}
        disabled={!available.includes(letter)}
        key={letter}
        letter={letter}
        onSelect={onSelect}
      />
    ))}
  </NativeView>
);

export type { LetterRailProps };
export { LetterRail };
