import { useEffect, useId, useState, useSyncExternalStore } from "react";
import { Platform } from "react-native";

import { useFocusLook } from "../focus-look";

type Ring = "actionFocus" | "contentPrimary" | "transparent";

// One ring at a time: while a control holds keyboard focus, a pointer draws no ring on another.
const holders = new Set<string>();
const listeners = new Set<() => void>();
const publish = () => {
  for (const listener of listeners) listener();
};
const subscribe = (listener: () => void) => {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
};
const held = () => holders.size > 0;

// The browser's own test of keyboard focus, so a click never draws the amber ring. A native view has
// no `matches`, and there focus only ever comes from a remote or a keyboard.
const fromKeyboard = (event: unknown) => {
  const target = (event as { target?: { matches?: (selector: string) => boolean } } | undefined)?.target;
  return target?.matches ? target.matches(":focus-visible") : true;
};

/**
 * Which ring a control draws: amber while keyboard or remote focus is on it, plain while a pointer
 * is over it, and none left behind by a mouse click. A pointer press on a control that holds keyboard
 * focus takes its amber ring away; the next key press brings it back.
 *
 * Spread `handlers` onto the pressable that takes the focus, and draw `ring` on its face. The
 * control turns the platform's own focus mark off, so this ring is the only one.
 */
const useRingFocus = (own: { onBlur?: () => void; onFocus?: (fromKeys: boolean) => void } = {}) => {
  const id = useId();
  const [hovered, setHovered] = useState(false);
  const [keyboard, setKeyboard] = useState(false);
  const { focused, onBlur, onFocus } = useFocusLook({
    onBlur: () => own.onBlur?.(),
    onFocus: (event: unknown) => {
      const fromKeys = fromKeyboard(event);
      setKeyboard(fromKeys);
      own.onFocus?.(fromKeys);
    },
  });
  const amber = focused && keyboard;
  const anyHeld = useSyncExternalStore(subscribe, held, held);

  // Released on unmount too, since a removed element sends no blur.
  useEffect(() => {
    if (!amber) return;
    holders.add(id);
    publish();
    return () => {
      holders.delete(id);
      publish();
    };
  }, [amber, id]);

  const ring: Ring = amber ? "actionFocus" : hovered && !anyHeld ? "contentPrimary" : "transparent";

  // The DOM's pointer and key events, which React Native's Pressable types leave out.
  const modality: object =
    Platform.OS === "web"
      ? { onKeyDownCapture: () => setKeyboard(true), onPointerDown: () => setKeyboard(false) }
      : {};

  return {
    handlers: {
      onBlur,
      onFocus,
      onHoverIn: () => setHovered(true),
      onHoverOut: () => setHovered(false),
      ...modality,
    },
    hovered,
    ring,
  };
};

export type { Ring };
export { useRingFocus };
