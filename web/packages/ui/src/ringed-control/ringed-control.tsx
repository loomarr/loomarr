import { Surface, useFocusLook } from "@loomarr/design-system";
import { type ReactNode, useState } from "react";
import { Platform, Pressable, type ViewStyle } from "react-native";

/**
 * ⚠ Not React Native's prop types: Storybook's docgen follows a prop's types into their imports and
 * cannot parse react-native's source. Anything else (a Tooltip's trigger handlers) passes through.
 */
interface RingedControlProps {
  "aria-expanded"?: boolean | undefined;
  onBlur?: (event: unknown) => void;
  onFocus?: (event: unknown) => void;
  onHoverIn?: (event: unknown) => void;
  onHoverOut?: (event: unknown) => void;
  onPress?: (() => void) | undefined;
  tabIndex?: 0 | -1;
  accessibilityLabel: string;
  children: ReactNode;
  /** Shows as on: what the control opens is open. */
  on?: boolean;
  /** The target's height and least width in points; 44 for a touch control. */
  size: number;
  /** Words drawn above the control while the keyboard or remote is on it. */
  focusLabel?: ReactNode;
  /** What the face stands at inside the target, where the target is taller than the drawing. */
  faceHeight?: number;
  /** The control's host node, so a toolbar can move focus to it. */
  ref?: (node: unknown) => void;
}

// The browser's own test of keyboard focus, so a click never draws the amber ring. A native view has
// no `matches`, and there focus only ever comes from a remote or a keyboard.
const fromKeyboard = (event: unknown) => {
  const target = (event as { target?: { matches?: (selector: string) => boolean } } | undefined)?.target;
  return target?.matches ? target.matches(":focus-visible") : true;
};

/**
 * A control of The Cross's watching chrome: a flat face with one ring. Keyboard and remote focus
 * draw it amber; a pointer over the control draws it plain.
 */
const RingedControl = ({
  children,
  faceHeight,
  focusLabel,
  on = false,
  onBlur,
  onFocus,
  onHoverIn,
  onHoverOut,
  size,
  ...rest
}: RingedControlProps) => {
  const [hovered, setHovered] = useState(false);
  const [keyboard, setKeyboard] = useState(false);
  const { focused, ...focusHandlers } = useFocusLook({
    onBlur: (event: unknown) => onBlur?.(event),
    onFocus: (event: unknown) => {
      setKeyboard(fromKeyboard(event));
      onFocus?.(event);
    },
  });
  const amber = focused && keyboard;
  const ring = amber ? "actionFocus" : hovered ? "contentPrimary" : "transparent";
  const height = faceHeight ?? size;

  return (
    <Pressable
      accessibilityRole="button"
      {...(rest as object)}
      {...focusHandlers}
      onHoverIn={(event) => {
        setHovered(true);
        onHoverIn?.(event);
      }}
      onHoverOut={(event) => {
        setHovered(false);
        onHoverOut?.(event);
      }}
      style={noOutline}
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
          backgroundColor={on || hovered ? "surfaceFocus" : "transparent"}
          borderColor={ring}
          borderRadius={8}
          borderWidth={2}
          flexDirection="row"
          gap={8}
          height={height}
          justifyContent="center"
          minWidth={size}
          paddingHorizontal={8}
          testID={`ring-${ring}`}
        >
          {children}
        </Surface>
      </Surface>
      {amber && focusLabel ? (
        <Surface
          backgroundColor="surfaceCanvas"
          borderRadius={6}
          borderWidth={0}
          bottom={size + 8}
          paddingHorizontal={8}
          paddingVertical={4}
          position="absolute"
        >
          {focusLabel}
        </Surface>
      ) : null}
    </Pressable>
  );
};

// The ring is the control's only focus and hover drawing, so the browser's outline is off.
const noOutline = Platform.OS === "web" ? ({ outlineStyle: "none" } as unknown as ViewStyle) : null;

export type { RingedControlProps };
export { RingedControl };
