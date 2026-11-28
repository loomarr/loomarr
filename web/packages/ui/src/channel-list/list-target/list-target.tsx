import { Surface, useFocusLook } from "@loomarr/design-system";
import { type ReactNode, useState } from "react";
import { type GestureResponderEvent, Platform, Pressable, type ViewStyle } from "react-native";

/**
 * What the list needs of a mounted target. ⚠ Not React Native's View type: Storybook's docgen follows
 * a prop's types into their imports and cannot parse react-native's source.
 */
interface TargetHandle {
  focus: () => void;
}

interface ListTargetProps {
  children: ReactNode;
  label: string;
  role: "option" | "tab" | "button";
  selected?: boolean | undefined;
  /** Whether this target carries the amber ring: the cursor, and only from keys or a remote. */
  ringed: boolean;
  /** Whether this target is its group's one Tab stop. */
  tabStop: boolean;
  onPress: (event: GestureResponderEvent) => void;
  /** Keyboard or remote focus landed here; a pointer's focus is not reported. */
  onKeyFocus?: (() => void) | undefined;
  onHoverIn?: (() => void) | undefined;
  onHoverOut?: (() => void) | undefined;
  targetRef?: ((node: TargetHandle | null) => void) | undefined;
  minHeight: number;
  style?: ViewStyle | undefined;
}

// A native view has no `matches`, and there focus only ever comes from a remote or a keyboard.
const fromKeys = (event: unknown) => {
  const target = (event as { target?: { matches?: (selector: string) => boolean } } | undefined)?.target;
  return target?.matches ? target.matches(":focus-visible") : true;
};

// The ring is the only focus drawing, so the browser's outline is off.
const noOutline = Platform.OS === "web" ? ({ outlineStyle: "none" } as unknown as ViewStyle) : null;

/** One row, tab or pinned channel: the amber ring for the cursor, a plain fill for a pointer. */
const ListTarget = ({
  children,
  label,
  role,
  selected,
  ringed,
  tabStop,
  onPress,
  onKeyFocus,
  onHoverIn,
  onHoverOut,
  targetRef,
  minHeight,
  style,
}: ListTargetProps) => {
  const [hovered, setHovered] = useState(false);
  const { focused: _, ...focusHandlers } = useFocusLook({
    onFocus: (event: unknown) => {
      if (fromKeys(event)) onKeyFocus?.();
    },
  });
  return (
    <Pressable
      accessibilityLabel={label}
      {...focusHandlers}
      onHoverIn={() => {
        setHovered(true);
        onHoverIn?.();
      }}
      onHoverOut={() => {
        setHovered(false);
        onHoverOut?.();
      }}
      onPress={onPress}
      ref={targetRef as never}
      role={role as never}
      style={[noOutline, style]}
      {...({ "aria-selected": selected, tabIndex: tabStop ? 0 : -1 } as object)}
    >
      <Surface
        backgroundColor={hovered ? "surfaceElevated" : "transparent"}
        borderColor={ringed ? "actionFocus" : "transparent"}
        borderRadius="control"
        borderWidth={2}
        flexDirection="row"
        alignItems="center"
        gap={12}
        minHeight={minHeight}
        paddingHorizontal={8}
        {...({ "data-ringed": ringed || undefined } as object)}
      >
        {children}
      </Surface>
    </Pressable>
  );
};

export type { TargetHandle };
export { ListTarget };
