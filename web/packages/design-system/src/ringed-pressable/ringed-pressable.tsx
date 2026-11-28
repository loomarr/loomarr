import type { ReactElement, ReactNode } from "react";

import { Surface } from "../primitives";
import { RingTarget, type RingTargetHandle, type RingTargetProps } from "../ring-target";
import { Text } from "../text";
import { Tooltip } from "../tooltip";

/** What a caller needs of a mounted control, to move focus to it. */
type RingedPressableHandle = RingTargetHandle;

interface RingedPressableProps
  extends Pick<
    RingTargetProps,
    | "accessibilityLabel"
    | "expanded"
    | "onBlur"
    | "onHoverIn"
    | "onHoverOut"
    | "onKeyboardFocus"
    | "onMouseDown"
    | "onPress"
    | "pointerTakesNoFocus"
    | "ref"
    | "role"
    | "selected"
    | "tabIndex"
  > {
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
  /** What the face stands at inside the target, where the target is taller than the drawing. */
  faceHeight?: number;
  /** Shows as on. */
  on?: boolean;
  onFocus?: () => void;
  /** The target's height and least width in points; 44 for a touch control. */
  size: number;
  /** Words naming the control. */
  tip?: string | undefined;
  /**
   * `hover` is a tooltip on hover and keyboard focus. `focus` draws the words above the control
   * while the keyboard or remote is on it, for a television, which has no hover.
   */
  tipOn?: "focus" | "hover";
}

/** A `RingTarget` with a flat face that draws its ring. */
const RingedPressable = ({
  align = "centre",
  children,
  faceHeight,
  fill = false,
  highlighted,
  on = false,
  size,
  tip,
  tipOn = "hover",
  ...target
}: RingedPressableProps) => (
  <RingTarget
    {...target}
    fill={fill}
    wrap={
      tip && tipOn === "hover"
        ? (control: ReactElement) => <Tooltip content={tip}>{control}</Tooltip>
        : undefined
    }
  >
    {(face) => {
      const hovered = highlighted ?? face.hovered;
      // A caller that says the control is not highlighted takes the pointer's plain ring away too.
      const ring = highlighted === false && face.ring === "contentPrimary" ? "transparent" : face.ring;
      return (
        <>
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
              justifyContent={align === "start" ? "flex-start" : "center"}
              minWidth={size}
              paddingHorizontal={8}
              testID={`ring-${ring}`}
            >
              {children}
            </Surface>
          </Surface>
          {face.keyboard && tip && tipOn === "focus" ? (
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
        </>
      );
    }}
  </RingTarget>
);

export type { RingedPressableHandle, RingedPressableProps };
export { RingedPressable };
