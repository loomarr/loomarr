import { isWeb, useTheme } from "@tamagui/core";
import type { CSSProperties, ForwardRefExoticComponent, ReactNode, RefAttributes } from "react";
import { createElement, forwardRef } from "react";
import { type GestureResponderEvent, Pressable } from "react-native";

import { useFocusLook } from "../focus-look";
import { spaceSteps } from "../tokens";

type MediaTileProps = {
  /** The picture: a served image, a frame with a badge laid over it. The tile clips it to its corners. */
  children: ReactNode;
  /**
   * The tile's accessible name. A tile whose name is better read from its own words (a still with a
   * visible title) or from a marker elsewhere on the screen leaves this out and names itself with
   * `aria-labelledby`, as a button would.
   */
  label?: string;
  "aria-controls"?: string;
  "aria-current"?: "true";
  "aria-describedby"?: string;
  "aria-expanded"?: boolean;
  "aria-labelledby"?: string;
  /** A 1.0 ratio, 16 / 9, a poster's 2 / 3: the shape of the picture when the tile has no height of its own. */
  aspectRatio?: number;
  /** The tile draws a hairline around the picture. Default on; a frame that already has an edge turns it off. */
  bordered?: boolean;
  /**
   * The tile is the current one of its set (the open preview, the segment the player is on): a
   * neutral ring outside the edge. Amber is focus, so a current tile is never mistaken for a focused one.
   */
  current?: boolean;
  disabled?: boolean;
  /**
   * Draws the focus ring inside the tile, for one that fills a frame which clips (the play target over
   * a preview, a segment in a bar): an outside ring would be cut off at the frame's edge.
   */
  focusInset?: boolean;
  height?: number | "100%";
  onPress?: (event: GestureResponderEvent) => void;
  /** `small` is 4 px, `medium` 6 px: the corner of the picture, as a thumbnail or a poster is rounded. */
  radius?: "medium" | "none" | "small";
  /**
   * `false` draws no fill behind the picture, for a tile that is a target laid over a picture of its
   * own (the play button over a preview frame) or over a fill the parent draws (a segment of a bar).
   */
  fill?: boolean;
  /** `warning` draws the edge in the warning colour: a still the reel cannot split. */
  tone?: "neutral" | "warning";
  title?: string;
  width?: number | "100%";
};

type MediaTileRef = HTMLButtonElement;

const radii = { medium: spaceSteps["1.5"], none: 0, small: spaceSteps["1"] } as const;

/**
 * A picture that is also a button: a clip's thumbnail that opens its preview, a poster that is picked
 * as a channel's icon, a still on a timeline. The tile is one control whose name is the caller's
 * `label`; what it draws is the picture, a hairline edge and the one amber focus ring every control
 * draws (§9), so a grid of tiles looks and focuses the same wherever it is.
 *
 * On the web it is a real `<button>` that forwards its ref and any prop a parent hands it, so it is a
 * Base UI Tooltip's trigger and a place to return focus to. On native it is a Pressable.
 */
const MediaTile: ForwardRefExoticComponent<MediaTileProps & RefAttributes<MediaTileRef>> = forwardRef<
  MediaTileRef,
  MediaTileProps
>((props, ref) => {
  const {
    aspectRatio,
    bordered = true,
    children,
    current = false,
    disabled = false,
    fill = true,
    focusInset = false,
    height,
    label,
    onPress,
    radius = "small",
    tone = "neutral",
    width,
    ...hostProps
  } = props;
  const theme = useTheme();
  const focus = useFocusLook<unknown>(
    hostProps as { onBlur?: (event: unknown) => void; onFocus?: (event: unknown) => void },
  );
  const borderRadius = radii[radius];
  const edgeColour = tone === "warning" ? theme.stateWarning.val : theme.borderDecorative.val;
  const background = fill ? theme.artworkPlaceholder.val : "transparent";

  if (!isWeb) {
    return (
      <Pressable
        accessibilityLabel={label}
        accessibilityRole="button"
        accessibilityState={{ disabled }}
        disabled={disabled}
        onPress={onPress}
        style={({ pressed }) => ({
          alignItems: "center",
          aspectRatio,
          backgroundColor: background,
          borderColor: focus.focused ? theme.actionFocus.val : edgeColour,
          borderRadius,
          borderWidth: focus.focused ? 2 : bordered ? 1 : 0,
          height,
          justifyContent: "center",
          opacity: pressed && !disabled ? 0.82 : 1,
          overflow: "hidden",
          width,
        })}
      >
        {children}
      </Pressable>
    );
  }

  const style: CSSProperties = {
    alignItems: "center",
    aspectRatio,
    background,
    border: bordered ? `1px solid ${edgeColour}` : 0,
    borderRadius,
    // The current ring sits outside the edge and the focus ring outside that, so both can show at once.
    boxShadow: current ? `0 0 0 2px ${theme.contentPrimary.val}` : undefined,
    boxSizing: "border-box",
    color: "inherit",
    cursor: disabled ? "default" : "pointer",
    display: "flex",
    flexShrink: 0,
    font: "inherit",
    height,
    justifyContent: "center",
    margin: 0,
    overflow: "hidden",
    padding: 0,
    position: "relative",
    textAlign: "left",
    width,
    ...(focus.focused
      ? {
          outline: `2px solid ${theme.actionFocus.val}`,
          outlineOffset: focusInset ? -2 : 2,
        }
      : { outline: "none" }),
  };

  // `hostProps` is whatever a parent hands the tile (a Tooltip trigger's hover, focus and describedby
  // handlers, a composed `className`): it reaches the button, with the tile's own focus tracking
  // chained over its `onBlur` and `onFocus`.
  return createElement(
    "button",
    {
      ...hostProps,
      "aria-label": label,
      disabled,
      onBlur: focus.onBlur,
      onClick: (event: GestureResponderEvent) => {
        (hostProps as { onClick?: (event: unknown) => void }).onClick?.(event);
        onPress?.(event);
      },
      onFocus: focus.onFocus,
      ref,
      style,
      type: "button",
    },
    children,
  );
});

MediaTile.displayName = "MediaTile";

export type { MediaTileProps };
export { MediaTile };
