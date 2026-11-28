import { View } from "@tamagui/core";
import { useState } from "react";
import { View as NativeView, Pressable, useWindowDimensions } from "react-native";

import { useFocusLook } from "../focus-look";
import { TamaguiText } from "../text-base";

// "#" stands for every title that starts with a digit or any other character. It leads, as such
// titles sort first.
const LETTERS = ["#", ..."ABCDEFGHIJKLMNOPQRSTUVWXYZ"];

// Every letter is a 24 by 24 px target, the minimum (WCAG 2.5.8). A window too short for all 27
// scrolls the rail rather than shrinking them.
const SIZE = 24;
const EDGES = 16;

type LetterRailProps = {
  /** Names the rail to a screen reader. */
  label?: string;
  /** The letters that have something to jump to; every other letter shows disabled. */
  available: readonly string[];
  /** The letter the reader is at, drawn raised. */
  current?: string;
  /** The letter still loading what it jumps to, drawn with a dashed edge. */
  busy?: string;
  onSelect: (letter: string) => void;
};

const Letter = ({
  letter,
  disabled,
  current,
  busy,
  onSelect,
}: {
  letter: string;
  disabled: boolean;
  current: boolean;
  busy: boolean;
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
      accessibilityState={{ disabled, busy }}
      aria-busy={busy || undefined}
      aria-current={current ? "true" : undefined}
      disabled={disabled}
      onBlur={focus.onBlur}
      onFocus={focus.onFocus}
      onHoverIn={() => setHovered(true)}
      onHoverOut={() => setHovered(false)}
      onPress={() => onSelect(letter)}
      // The amber border below is the focus look; forced colours turn the transparent outline visible.
      style={{ outlineColor: "transparent", outlineStyle: "solid", outlineWidth: focus.focused ? 2 : 0 }}
    >
      <View
        alignItems="center"
        backgroundColor={current || (hovered && !disabled) ? "surfaceRaised" : "transparent"}
        borderColor={focus.focused ? "actionFocus" : busy ? "contentSecondary" : "transparent"}
        borderRadius="control"
        borderStyle={busy ? "dashed" : "solid"}
        borderWidth={busy && !focus.focused ? 1 : 2}
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
const LetterRail = ({ label = "Jump to a letter", available, current, busy, onSelect }: LetterRailProps) => {
  const maxHeight = useWindowDimensions().height - EDGES;
  return (
    <NativeView aria-label={label} role="navigation" style={{ maxHeight, overflow: "scroll" }}>
      {LETTERS.map((letter) => (
        <Letter
          busy={letter === busy}
          current={letter === current}
          disabled={!available.includes(letter)}
          key={letter}
          letter={letter}
          onSelect={onSelect}
        />
      ))}
    </NativeView>
  );
};

export type { LetterRailProps };
export { LetterRail };
