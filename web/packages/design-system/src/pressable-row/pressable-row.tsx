import { isWeb, View } from "@tamagui/core";
import type {
  ComponentProps,
  ComponentRef,
  ForwardRefExoticComponent,
  ReactElement,
  ReactNode,
  RefAttributes,
} from "react";
import { cloneElement, forwardRef, isValidElement, useState } from "react";
import { Pressable } from "react-native";

import { useFocusLook } from "../focus-look";
import { semanticSpace } from "../tokens";

/** What a router link reports about itself, which a link row reads to draw the current page. */
type PressableRowLinkState = { isActive: boolean };

type PressableRowProps = Omit<ComponentProps<typeof Pressable>, "children" | "style"> & {
  /**
   * The row's content, laid out as a row: a thumbnail, a block of text, a trailing status. Given as
   * a function it receives the link's state, so a nav row can read whether its route is the
   * current one (`isActive`); a plain row is never active.
   */
  children: ReactNode | ((state: PressableRowLinkState) => ReactNode);
  /**
   * The row is the current one of its set (the open item, the highlighted option): an elevated
   * fill. Given as a function it receives the link's state, to follow the route (`isActive`) or to
   * override it.
   */
  selected?: boolean | ((state: PressableRowLinkState) => boolean);
  /** A hairline under the row, for rows stacked in a list. */
  divided?: boolean;
  /**
   * Web: the DOM mousedown on the row, which react-native-web forwards to the element. An option of
   * a combobox listbox calls `preventDefault` here so the input keeps focus while the row is
   * pressed. Native has no mousedown and ignores it.
   */
  onMouseDown?: (event: { preventDefault: () => void }) => void;
  /**
   * Web: the element the row draws as, so a router's `Link` is the row and navigates without a page
   * load: `render={<Link to="/guide" />}`. The element keeps its own semantics (a link stays a
   * link), so leave `accessibilityRole` out unless the row needs another. Its `onClick`, `style` and
   * `className` stay and are chained with the row's. Native has no such element and draws the
   * pressable button.
   */
  render?: ReactElement<Record<string, unknown>>;
};

type PressableRowRef = ComponentRef<typeof Pressable>;

/**
 * A whole row that opens or picks something: the list of incoming clips, a log line, a search
 * result, a place in a picker. The row is one button (or the role the caller names, such as
 * `option`), so its name is its content; content is laid out by the caller. Hover and the current
 * row take the elevated fill; focus is the one amber border, drawn inside the row so a list does not
 * shift. With `render` it is a link instead, drawn the same way.
 */
const PressableRow: ForwardRefExoticComponent<PressableRowProps & RefAttributes<PressableRowRef>> =
  forwardRef<PressableRowRef, PressableRowProps>(
    (
      {
        accessibilityRole,
        accessibilityState,
        children,
        disabled = false,
        divided = false,
        onBlur,
        onFocus,
        render,
        selected = false,
        ...props
      },
      ref,
    ) => {
      const focusLook = useFocusLook({ onBlur, onFocus });
      const [hovered, setHovered] = useState(false);
      const role = accessibilityRole ?? (render ? undefined : "button");
      const isSelected = (state: PressableRowLinkState) =>
        typeof selected === "function" ? selected(state) : selected;
      const drawn = (state: PressableRowLinkState) => (
        <View
          backgroundColor={isSelected(state) || (hovered && !disabled) ? "$surfaceElevated" : "transparent"}
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
            {typeof children === "function" ? children(state) : children}
          </View>
        </View>
      );
      const idle: PressableRowLinkState = { isActive: false };

      if (isWeb && render && isValidElement(render)) {
        // The host element takes DOM names, not Pressable's: a press is a click, and a label is an
        // `aria-label`. A host's own `onClick` and `style` stay and are chained with these.
        const { accessibilityLabel, onMouseDown, onPress, testID, ...rest } = props as typeof props & {
          onMouseDown?: (event: { preventDefault: () => void }) => void;
        };
        const own = render.props as {
          onBlur?: (event: unknown) => void;
          onClick?: (event: unknown) => void;
          onFocus?: (event: unknown) => void;
          onMouseEnter?: (event: unknown) => void;
          onMouseLeave?: (event: unknown) => void;
          style?: object;
        };
        const aria = Object.fromEntries(Object.entries(rest).filter(([name]) => name.startsWith("aria-")));
        return cloneElement(
          render,
          {
            ...aria,
            // The router's own link already sets `aria-current` from its route.
            ...(selected === true && role === "link" ? { "aria-current": "page" } : {}),
            "aria-disabled": disabled || undefined,
            "aria-label": accessibilityLabel ?? (aria["aria-label"] as string | undefined),
            "aria-selected":
              role === undefined || role === "button" || role === "link" ? undefined : isSelected(idle),
            "data-testid": testID,
            onBlur: (event: unknown) => {
              focusLook.onBlur(event as never);
              own.onBlur?.(event);
            },
            onClick: (event: unknown) => {
              if (disabled) {
                // A disabled link goes nowhere.
                (event as { preventDefault?: () => void }).preventDefault?.();
                return;
              }
              own.onClick?.(event);
              onPress?.(event as never);
            },
            onFocus: (event: unknown) => {
              focusLook.onFocus(event as never);
              own.onFocus?.(event);
            },
            onMouseDown,
            onMouseEnter: (event: unknown) => {
              setHovered(true);
              own.onMouseEnter?.(event);
            },
            onMouseLeave: (event: unknown) => {
              setHovered(false);
              own.onMouseLeave?.(event);
            },
            ref,
            role: accessibilityRole,
            style: {
              boxSizing: "border-box",
              color: "inherit",
              cursor: disabled ? "default" : "pointer",
              display: "block",
              opacity: disabled ? 0.55 : undefined,
              outline: "none",
              textDecoration: "none",
              width: "100%",
              ...own.style,
            },
            tabIndex: disabled ? -1 : undefined,
          },
          // A router link calls a function child with its state, which is how the row learns its
          // route is the current one. Any other element takes the drawn row as it is.
          typeof children === "function" || typeof selected === "function"
            ? (((state: PressableRowLinkState) => drawn(state ?? idle)) as unknown as ReactNode)
            : drawn(idle),
        ) as ReactElement;
      }

      return (
        <Pressable
          accessibilityRole={role}
          // `selected` is announced only for a role that has it (option, tab, row): aria-selected on a
          // plain button is invalid, so a button row shows the fill alone.
          accessibilityState={{ ...accessibilityState, disabled }}
          aria-selected={role === "button" ? undefined : isSelected(idle)}
          disabled={disabled}
          onBlur={focusLook.onBlur}
          onFocus={focusLook.onFocus}
          onHoverIn={() => setHovered(true)}
          onHoverOut={() => setHovered(false)}
          ref={ref}
          style={({ pressed }) => ({ opacity: disabled ? 0.55 : pressed ? 0.82 : 1, width: "100%" })}
          {...props}
        >
          {drawn(idle)}
        </Pressable>
      );
    },
  );

PressableRow.displayName = "PressableRow";

export type { PressableRowProps };
export { PressableRow };
