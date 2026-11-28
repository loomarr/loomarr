import { type ReactNode, type Ref, useEffect, useRef } from "react";
import { Platform, Pressable, type ViewStyle } from "react-native";

import { type Ring, useRingFocus } from "../ring-focus";

/** What a caller needs of a mounted control, to move focus to it. */
interface RingTargetHandle {
  focus: () => void;
}

/** What the caller draws its face from. */
interface RingTargetFace {
  /** The ring's colour token: amber for keyboard or remote focus, plain for a pointer, or none. */
  ring: Ring;
  hovered: boolean;
  /** True while the keyboard or a remote holds the control. */
  keyboard: boolean;
}

interface RingTargetProps {
  accessibilityLabel: string;
  /** The control's face, at whatever shape and size the caller needs. */
  children: (face: RingTargetFace) => ReactNode;
  onBlur?: () => void;
  /** `fromKeys` is false when a pointer gave the focus. */
  onFocus?: (fromKeys: boolean) => void;
  onHoverIn?: () => void;
  onHoverOut?: () => void;
  /** The keyboard or remote focus came (`true`) or went. Focus a pointer gave never reports. */
  onKeyboardFocus?: (focused: boolean) => void;
  onPress?: () => void;
  /** Web: a pointer press takes no focus, so focus stays where it was. */
  pointerTakesNoFocus?: boolean;
  ref?: Ref<RingTargetHandle>;
  role?: "button" | "option" | "tab";
  /** Whether an option or tab is the selected one. A button has no such state. */
  selected?: boolean;
  tabIndex?: 0 | -1;
  testID?: string;
}

// The caller's ring is the control's only focus and hover drawing, so the platform's own mark is off.
// React Native's outline types stop at `solid`; react-native-web writes `none` through.
const noOutline: ViewStyle | null =
  Platform.OS === "web" ? { outlineStyle: "none" as never, outlineWidth: 0 } : null;

/**
 * A pressable with no face of its own and the one ring of `useRingFocus`, for a control whose shape
 * `RingedPressable` cannot take. The caller draws the ring it is given.
 */
const RingTarget = ({
  accessibilityLabel,
  children,
  onBlur,
  onFocus,
  onHoverIn,
  onHoverOut,
  onKeyboardFocus,
  onPress,
  pointerTakesNoFocus = false,
  ref,
  role = "button",
  selected,
  tabIndex,
  testID,
}: RingTargetProps) => {
  const { handlers, hovered, ring } = useRingFocus({
    ...(onBlur ? { onBlur } : {}),
    ...(onFocus ? { onFocus } : {}),
  });
  const keyboard = ring === "actionFocus";

  // Reported after the commit, and once more as `false` if the control leaves while it holds it.
  const reported = useRef(false);
  const report = useRef(onKeyboardFocus);
  report.current = onKeyboardFocus;
  useEffect(() => {
    if (reported.current === keyboard) return;
    reported.current = keyboard;
    report.current?.(keyboard);
  });
  useEffect(
    () => () => {
      if (reported.current) report.current?.(false);
    },
    [],
  );

  return (
    <Pressable
      accessibilityLabel={accessibilityLabel}
      aria-selected={role === "button" ? undefined : (selected ?? false)}
      {...handlers}
      onHoverIn={() => {
        handlers.onHoverIn();
        onHoverIn?.();
      }}
      onHoverOut={() => {
        handlers.onHoverOut();
        onHoverOut?.();
      }}
      onPress={onPress}
      ref={ref as Ref<never>}
      role={role}
      style={noOutline}
      testID={testID}
      {...(tabIndex === undefined ? {} : { tabIndex })}
      {...(Platform.OS === "web" && pointerTakesNoFocus
        ? ({ onMouseDown: (event: { preventDefault: () => void }) => event.preventDefault() } as object)
        : {})}
    >
      {children({ hovered, keyboard, ring })}
    </Pressable>
  );
};

export type { RingTargetFace, RingTargetHandle, RingTargetProps };
export { RingTarget };
