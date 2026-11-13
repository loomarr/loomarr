import { isWeb, useTheme } from "@tamagui/core";
import type { CSSProperties, ReactNode } from "react";
import { createElement } from "react";

import { useFocusLook } from "../focus-look";
import { TamaguiText } from "../text-base";

type InlineActionProps = {
  /** The label, sentence case: "Follow installation geography". It reads as part of the sentence. */
  children: ReactNode;
  /** Names the action when the label alone is not enough ("Unlock the library path to edit it"). */
  "aria-label"?: string;
  disabled?: boolean;
  onPress: () => void;
};

/**
 * An action written into a line of prose: the "Use any era" after a hint, the "Edit" that ends a
 * sentence. It is a button that keeps inline flow, so it wraps with the words around it and takes
 * their size, weight and line height; what it adds is the underline that says it is pressable, the
 * primary text colour, and the one amber outline for focus (§9). Beside a block of its own, where it
 * stands alone on a line, use `Action layout="text"`, which draws the same look as a box.
 *
 * On the web it is a real `<button>` (Enter and Space, a name, a focus stop) with the 24 px touch
 * target a touch screen needs (#1785); on native, text that takes the press.
 */
const InlineAction = ({
  "aria-label": ariaLabel,
  children,
  disabled = false,
  onPress,
}: InlineActionProps) => {
  const theme = useTheme();
  const focus = useFocusLook();
  const look = {
    color: theme.contentPrimary.val,
    textDecorationLine: "underline",
    ...(disabled && { opacity: 0.5 }),
  } as const;

  if (!isWeb) {
    return (
      <TamaguiText
        accessibilityLabel={ariaLabel}
        accessibilityRole="button"
        accessibilityState={{ disabled }}
        color="contentPrimary"
        disabled={disabled}
        onPress={onPress}
        opacity={look.opacity}
        textDecorationLine="underline"
      >
        {children}
      </TamaguiText>
    );
  }

  const style: CSSProperties = {
    // The surrounding sentence's own type: a browser button would otherwise reset to the system face.
    background: "none",
    border: 0,
    borderRadius: 2,
    color: look.color,
    cursor: disabled ? "default" : "pointer",
    // Inline, not the browser button's inline-block, so a long label wraps with the words around it.
    display: "inline",
    font: "inherit",
    letterSpacing: "inherit",
    margin: 0,
    opacity: disabled ? 0.5 : undefined,
    padding: 0,
    textDecorationLine: "underline",
    textUnderlineOffset: 2,
    ...(focus.focused
      ? { outline: `2px solid ${theme.actionFocus.val}`, outlineOffset: 2 }
      : { outline: "none" }),
  };

  return createElement(
    "button",
    {
      "aria-label": ariaLabel,
      "data-touch-target": true,
      disabled,
      onBlur: focus.onBlur,
      onClick: onPress,
      onFocus: focus.onFocus,
      style,
      type: "button",
    },
    children,
  );
};

export type { InlineActionProps };
export { InlineAction };
