import { View } from "@tamagui/core";
import type {
  ComponentProps,
  ComponentRef,
  ForwardRefExoticComponent,
  ReactNode,
  RefAttributes,
} from "react";
import { forwardRef, useState } from "react";
import { Pressable } from "react-native";

import { useFocusLook } from "../focus-look";
import { semanticSpace } from "../tokens";

type PressableRowProps = Omit<ComponentProps<typeof Pressable>, "children" | "style"> & {
  /** The row's content, laid out as a row: a thumbnail, a block of text, a trailing status. */
  children: ReactNode;
  /** The row is the current one of its set (the open item, the highlighted option): an elevated fill. */
  selected?: boolean;
  /** A hairline under the row, for rows stacked in a list. */
  divided?: boolean;
};

type PressableRowRef = ComponentRef<typeof Pressable>;

/**
 * A whole row that opens or picks something: the list of incoming clips, a log line, a search
 * result, a place in a picker. The row is one button (or the role the caller names, such as
 * `option`), so its name is its content; content is laid out by the caller. Hover and the current
 * row take the elevated fill; focus is the one amber border, drawn inside the row so a list does not
 * shift.
 */
const PressableRow: ForwardRefExoticComponent<PressableRowProps & RefAttributes<PressableRowRef>> =
  forwardRef<PressableRowRef, PressableRowProps>(
    (
      {
        accessibilityRole = "button",
        accessibilityState,
        children,
        disabled = false,
        divided = false,
        onBlur,
        onFocus,
        selected = false,
        ...props
      },
      ref,
    ) => {
      const focusLook = useFocusLook({ onBlur, onFocus });
      const [hovered, setHovered] = useState(false);
      const filled = selected || (hovered && !disabled);
      return (
        <Pressable
          accessibilityRole={accessibilityRole}
          accessibilityState={{ ...accessibilityState, disabled, selected }}
          disabled={disabled}
          onBlur={focusLook.onBlur}
          onFocus={focusLook.onFocus}
          onHoverIn={() => setHovered(true)}
          onHoverOut={() => setHovered(false)}
          ref={ref}
          style={({ pressed }) => ({ opacity: disabled ? 0.55 : pressed ? 0.82 : 1, width: "100%" })}
          {...props}
        >
          <View
            backgroundColor={filled ? "$surfaceElevated" : "transparent"}
            borderBottomColor="$borderDecorative"
            borderBottomWidth={divided ? 1 : 0}
          >
            <View
              alignItems="center"
              borderColor={focusLook.focused ? "$actionFocus" : "transparent"}
              borderWidth={2}
              flexDirection="row"
              gap={semanticSpace.control}
              minWidth={0}
              paddingHorizontal={semanticSpace.control - 2}
              paddingVertical={semanticSpace.inline}
            >
              {children}
            </View>
          </View>
        </Pressable>
      );
    },
  );

PressableRow.displayName = "PressableRow";

export type { PressableRowProps };
export { PressableRow };
