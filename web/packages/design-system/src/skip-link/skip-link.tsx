import { isWeb, useTheme } from "@tamagui/core";
import type { CSSProperties, ReactNode } from "react";
import { useState } from "react";

import { Text } from "../text";

type SkipLinkProps = {
  /** The label, sentence case: "Skip to content". */
  children: ReactNode;
  /** The in-page address of the content it jumps to: `#main-content`. */
  href: string;
};

// Visually hidden, but still the first stop of the Tab order and still read by a screen reader: the
// standard clip-to-a-pixel recipe, not `display: none`, which would take it out of both.
const hidden: CSSProperties = {
  border: 0,
  clip: "rect(0, 0, 0, 0)",
  height: 1,
  margin: -1,
  overflow: "hidden",
  padding: 0,
  position: "absolute",
  whiteSpace: "nowrap",
  width: 1,
};

/**
 * "Skip to content", the first Tab stop of a page whose chrome puts a long run of controls before its
 * content (WCAG 2.4.1). It is invisible until it holds focus, then reveals itself at the top left of
 * the window as the amber primary-action chip, above everything. Web only: a phone and a remote
 * have no Tab key and draw nothing.
 */
const SkipLink = ({ children, href }: SkipLinkProps) => {
  const theme = useTheme();
  const [focused, setFocused] = useState(false);
  if (!isWeb) return null;

  const revealed: CSSProperties = {
    background: theme.actionPrimary.val,
    borderRadius: 6,
    color: theme.contentInverse.val,
    left: 8,
    padding: "8px 12px",
    position: "absolute",
    top: 8,
    zIndex: 50,
  };

  return (
    <Text
      as="a"
      // `href` is the host element's own attribute, which Text's props do not name.
      {...({ href } as object)}
      onBlur={() => setFocused(false)}
      onFocus={() => setFocused(true)}
      strong
      style={focused ? revealed : hidden}
      textRole="compact"
    >
      {children}
    </Text>
  );
};

export type { SkipLinkProps };
export { SkipLink };
