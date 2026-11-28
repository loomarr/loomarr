import type { ReactNode, Ref } from "react";
import { Platform, Pressable, type ViewStyle } from "react-native";

import { Surface } from "../primitives";
import { useRingFocus } from "../ring-focus";
import { Text } from "../text";
import { Tooltip } from "../tooltip";

/** What a caller needs of a mounted control, to move focus to it. */
interface RingedPressableHandle {
  focus: () => void;
}

interface RingedPressableProps {
  accessibilityLabel: string;
  children: ReactNode;
  /** Whether what the control opens is open; left out on a control that opens nothing. */
  expanded?: boolean | undefined;
  /** What the face stands at inside the target, where the target is taller than the drawing. */
  faceHeight?: number;
  /** Shows as on. */
  on?: boolean;
  onBlur?: () => void;
  onFocus?: () => void;
  onPress?: (() => void) | undefined;
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

/** A pressable with a flat face and the one ring of `useRingFocus`. */
const RingedPressable = ({
  accessibilityLabel,
  children,
  expanded,
  faceHeight,
  on = false,
  onBlur,
  onFocus,
  onPress,
  ref,
  size,
  tabIndex,
  tip,
  tipOn = "hover",
}: RingedPressableProps) => {
  const { handlers, hovered, ring } = useRingFocus({
    ...(onBlur ? { onBlur } : {}),
    ...(onFocus ? { onFocus } : {}),
  });

  const control = (
    <Pressable
      accessibilityLabel={accessibilityLabel}
      accessibilityRole="button"
      aria-expanded={expanded}
      {...handlers}
      onPress={onPress}
      ref={ref as Ref<never>}
      style={noOutline}
      {...(tabIndex === undefined ? {} : { tabIndex })}
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
          height={faceHeight ?? size}
          justifyContent="center"
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
