import { type ReactNode, type Ref, useEffect, useRef } from "react";
import { type GestureResponderEvent, Platform, Pressable, type ViewStyle } from "react-native";

import { Surface } from "../primitives";
import { useRingFocus } from "../ring-focus";
import { Text } from "../text";
import { Tooltip } from "../tooltip";

/** What a caller needs of a mounted control, to move focus to it. */
interface RingedPressableHandle {
  focus: () => void;
}

interface RingedPressableProps {
  /** The control's name. Left out, it is named by the words it holds, as an option often is. */
  accessibilityLabel?: string;
  /** Where the face lays its content: centred, as a control's icon, or from the leading edge. */
  align?: "centre" | "start";
  children: ReactNode;
  /**
   * Takes the whole width of its parent, as a row of a list does, and as much height as its content
   * needs, `size` at least.
   */
  fill?: boolean;
  /**
   * Draws the hover fill by the caller's word in place of the pointer's: for a caller that tracks
   * hover itself, or marks a highlighted option. Left out, the pointer decides.
   */
  highlighted?: boolean;
  /** Web: the DOM mousedown; `preventDefault` there keeps focus where it is. Native ignores it. */
  onMouseDown?: (event: { preventDefault: () => void }) => void;
  /**
   * Web: a pointer press takes no focus, so focus stays where it was and a key pressed after a
   * click is not the control's. Keyboard and remote focus are unchanged. Off unless asked for.
   */
  pointerTakesNoFocus?: boolean;
  /** Called beside the control's own hover drawing, never in place of it. */
  onHoverIn?: () => void;
  onHoverOut?: () => void;
  /** What the control is to assistive technology. */
  role?: "button" | "option" | "tab";
  /** Whether an option or tab is the selected one. A button has no such state. */
  selected?: boolean;
  /** Whether what the control opens is open; left out on a control that opens nothing. */
  expanded?: boolean | undefined;
  /** What the face stands at inside the target, where the target is taller than the drawing. */
  faceHeight?: number;
  /** Shows as on. */
  on?: boolean;
  onBlur?: () => void;
  onFocus?: () => void;
  /**
   * The keyboard or remote focus came (`true`) or went: the amber ring's coming and going. Focus a
   * pointer gave never reports. On the web a pointer press on a control that holds it reports
   * `false`; a native client has no pointer to press with.
   */
  onKeyboardFocus?: (focused: boolean) => void;
  /** The press, with where it came from: a pointer's carries a place, a key's does not. */
  onPress?: ((event: GestureResponderEvent) => void) | undefined;
  ref?: Ref<RingedPressableHandle>;
  /** The target's height and least width in points; 44 for a touch control. */
  size: number;
  tabIndex?: 0 | -1;
  /** Words naming the control. */
  tip?: string | undefined;
  /**
   * `hover` is a tooltip on hover and keyboard focus. `focus` draws the words above the control
   * while the keyboard or remote is on it, for a television, which has no hover.
   */
  tipOn?: "focus" | "hover";
}

// The ring is the control's only focus and hover drawing, so the platform's own mark is off. React
// Native's outline types stop at `solid`; react-native-web writes `none` through.
const noOutline: ViewStyle | null =
  Platform.OS === "web" ? { outlineStyle: "none" as never, outlineWidth: 0 } : null;
const fillOutline: ViewStyle = { ...noOutline, width: "100%" };

/** A pressable with a flat face and the one ring of `useRingFocus`. */
const RingedPressable = ({
  accessibilityLabel,
  align = "centre",
  children,
  expanded,
  faceHeight,
  fill = false,
  highlighted,
  on = false,
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
  size,
  tabIndex,
  tip,
  tipOn = "hover",
}: RingedPressableProps) => {
  const {
    handlers,
    hovered: pointed,
    ring: drawn,
  } = useRingFocus({
    ...(onBlur ? { onBlur } : {}),
    ...(onFocus ? { onFocus } : {}),
  });
  const hovered = highlighted ?? pointed;
  // A caller that says the control is not highlighted takes the pointer's plain ring away too.
  const ring = highlighted === false && drawn === "contentPrimary" ? "transparent" : drawn;
  const start = align === "start";

  // Reported after the commit, and once more as `false` if the control leaves while it holds it.
  const keyboard = drawn === "actionFocus";
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
      {/* The target is `size` high; the face inside it may be drawn shorter. */}
      <Surface
        backgroundColor="transparent"
        borderWidth={0}
        justifyContent="center"
        minHeight={size}
        minWidth={size}
      >
        <Surface
          alignItems="center"
          // The hover fill is the lighter step of the two, so the live colour still reads on it.
          backgroundColor={on ? "surfaceFocus" : hovered ? "surfaceElevated" : "transparent"}
          borderColor={ring}
          borderRadius={8}
          borderWidth={2}
          flexDirection="row"
          gap={8}
          height={fill ? undefined : (faceHeight ?? size)}
          minHeight={fill ? size : undefined}
          justifyContent={start ? "flex-start" : "center"}
          minWidth={size}
          paddingHorizontal={8}
          testID={`ring-${ring}`}
        >
          {children}
        </Surface>
      </Surface>
      {ring === "actionFocus" && tip && tipOn === "focus" ? (
        <Surface
          backgroundColor="surfaceCanvas"
          borderRadius={6}
          borderWidth={0}
          bottom={size + 8}
          paddingHorizontal={8}
          paddingVertical={4}
          position="absolute"
        >
          <Text textRole="label">{tip}</Text>
        </Surface>
      ) : null}
    </Pressable>
  );

  return tip && tipOn === "hover" ? <Tooltip content={tip}>{control}</Tooltip> : control;
};

export type { RingedPressableHandle, RingedPressableProps };
export { RingedPressable };
