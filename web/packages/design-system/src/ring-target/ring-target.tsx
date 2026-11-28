import { type ReactElement, type ReactNode, type Ref, useEffect, useRef } from "react";
import { type GestureResponderEvent, Platform, Pressable, type ViewStyle } from "react-native";

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
  /** The control's name. Left out, it is named by the words it holds, as an option often is. */
  accessibilityLabel?: string;
  /** The control's face, at whatever shape and size the caller needs. */
  children: (face: RingTargetFace) => ReactNode;
  /** Whether what the control opens is open; left out on a control that opens nothing. */
  expanded?: boolean | undefined;
  /** Takes the whole width of its parent, as a row of a list does. */
  fill?: boolean;
  onBlur?: () => void;
  /** `fromKeys` is false when a pointer gave the focus. */
  onFocus?: (fromKeys: boolean) => void;
  /** Called beside the control's own hover drawing, never in place of it. */
  onHoverIn?: () => void;
  onHoverOut?: () => void;
  /**
   * The keyboard or remote focus came (`true`) or went: the amber ring's coming and going. Focus a
   * pointer gave never reports. On the web a pointer press on a control that holds it reports
   * `false`; a native client has no pointer to press with.
   */
  onKeyboardFocus?: (focused: boolean) => void;
  /** Web: the DOM mousedown; `preventDefault` there keeps focus where it is. Native ignores it. */
  onMouseDown?: (event: { preventDefault: () => void }) => void;
  /** The press, with where it came from: a pointer's carries a place, a key's does not. */
  onPress?: ((event: GestureResponderEvent) => void) | undefined;
  /**
   * Web: a pointer press takes no focus, so focus stays where it was and a key pressed after a
   * click is not the control's. Keyboard and remote focus are unchanged. Off unless asked for.
   */
  pointerTakesNoFocus?: boolean;
  ref?: Ref<RingTargetHandle>;
  /** What the control is to assistive technology. */
  role?: "button" | "option" | "tab";
  /** Whether an option or tab is the selected one. A button has no such state. */
  selected?: boolean;
  tabIndex?: 0 | -1;
  testID?: string;
  /**
   * Puts something around the control that has to be handed the pressable itself, such as a tooltip.
   */
  wrap?: ((control: ReactElement) => ReactNode) | undefined;
}

// The caller's ring is the control's only focus and hover drawing, so the platform's own mark is off.
// React Native's outline types stop at `solid`; react-native-web writes `none` through.
const noOutline: ViewStyle | null =
  Platform.OS === "web" ? { outlineStyle: "none" as never, outlineWidth: 0 } : null;
const fillOutline: ViewStyle = { ...noOutline, width: "100%" };

/**
 * A pressable with no face of its own and the one ring of `useRingFocus`. The caller draws the ring
 * it is given; `RingedPressable` is this control with a fixed flat face.
 */
const RingTarget = ({
  accessibilityLabel,
  children,
  expanded,
  fill = false,
  onBlur,
  onFocus,
  onHoverIn,
  onHoverOut,
  onKeyboardFocus,
  onMouseDown,
  onPress,
  pointerTakesNoFocus = false,
  ref,
  role = "button",
  selected,
  tabIndex,
  testID,
  wrap,
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

  const control = (
    <Pressable
      accessibilityLabel={accessibilityLabel}
      aria-expanded={expanded}
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
      style={fill ? fillOutline : noOutline}
      testID={testID}
      {...(tabIndex === undefined ? {} : { tabIndex })}
      {...(Platform.OS === "web" && (onMouseDown || pointerTakesNoFocus)
        ? ({
            onMouseDown: (event: { preventDefault: () => void }) => {
              if (pointerTakesNoFocus) event.preventDefault();
              onMouseDown?.(event);
            },
          } as object)
        : {})}
    >
      {children({ hovered, keyboard, ring })}
    </Pressable>
  );

  return wrap ? wrap(control) : control;
};

export type { RingTargetFace, RingTargetHandle, RingTargetProps };
export { RingTarget };
