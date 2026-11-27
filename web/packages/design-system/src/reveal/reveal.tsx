import { isWeb } from "@tamagui/core";
import type { ComponentProps } from "react";
import { useState } from "react";

import { useReducedMotionPreference } from "../motion/use-reduced-motion";
import { Stack } from "../stack";

// StackProps is a union (a form Stack takes form attributes), and a plain Omit would collapse it.
type OmitFrom<Props, Keys extends PropertyKey> = Props extends unknown ? Omit<Props, Keys> : never;

type RevealProps = OmitFrom<ComponentProps<typeof Stack>, "opacity" | "pointerEvents"> & {
  /**
   * Whether the row it belongs to is hovered or holds focus (`useRevealOnHover`). Hidden, it is still
   * in the tree, so a screen reader reads it and Tab reaches a control inside it; it only draws
   * nothing and takes no pointer.
   */
  shown: boolean;
};

/** What `useRevealOnHover` hands the row: spread onto the element that is the row. */
type RevealRowProps = {
  onBlur: (event: { currentTarget: unknown; relatedTarget: unknown }) => void;
  onFocus: () => void;
  onMouseEnter: () => void;
  onMouseLeave: () => void;
};

const FADE_MS = 150;

/**
 * Hover or focus-within, for a row that reveals something (an audit line under a field, a play disc
 * over a picture). Spread `rowProps` on the row's element and give `shown` to its `Reveal`. Focus
 * counts as hover, so a keyboard reader tabbing into the row sees what a pointer sees, and moving
 * focus between two things inside the row does not hide it.
 */
const useRevealOnHover = () => {
  const [hovered, setHovered] = useState(false);
  const [focused, setFocused] = useState(false);
  const rowProps: RevealRowProps = {
    onBlur: (event) => {
      const row = event.currentTarget as { contains?: (node: unknown) => boolean };
      // Focus moving to another part of the same row is still the row holding focus.
      if (event.relatedTarget && row.contains?.(event.relatedTarget)) return;
      setFocused(false);
    },
    onFocus: () => setFocused(true),
    onMouseEnter: () => setHovered(true),
    onMouseLeave: () => setHovered(false),
  };
  return { rowProps, shown: hovered || focused };
};

/**
 * A child that fades in while its row is hovered or holds focus: the "changed by" line under a
 * setting, the play disc on a clip. It takes a Stack's props, so it can be the element itself
 * (`render="p"`) and carry its own layout. The fade is dropped for a reader who asks for less motion.
 */
const Reveal = ({ children, shown, style, ...rest }: RevealProps) => {
  const reduced = useReducedMotionPreference();
  return (
    <Stack
      {...rest}
      opacity={shown ? 1 : 0}
      pointerEvents={shown ? "auto" : "none"}
      style={{
        ...(isWeb && !reduced ? { transition: `opacity ${FADE_MS}ms` } : {}),
        ...(style as object),
      }}
    >
      {children}
    </Stack>
  );
};

export type { RevealProps, RevealRowProps };
export { Reveal, useRevealOnHover };
