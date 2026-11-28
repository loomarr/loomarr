import type { ReactNode } from "react";
import { Animated, type LayoutChangeEvent, Platform, type ViewStyle } from "react-native";

import { brandChroma, playerChrome, semanticColors } from "../tokens";

/** Points, or a share of the positioned parent. */
type LayerLength = number | `${number}%`;

type LayerColor =
  | "actionDisabled"
  | "contentSecondary"
  | "playerSurface"
  | `chroma${0 | 1 | 2 | 3 | 4 | 5 | 6}`;

const colors: Record<LayerColor, string> = {
  actionDisabled: semanticColors.action.disabled,
  chroma0: brandChroma[0],
  chroma1: brandChroma[1],
  chroma2: brandChroma[2],
  chroma3: brandChroma[3],
  chroma4: brandChroma[4],
  chroma5: brandChroma[5],
  chroma6: brandChroma[6],
  contentSecondary: semanticColors.content.secondary,
  playerSurface: playerChrome.surface,
};

type LayerProps = {
  "aria-hidden"?: boolean;
  "aria-label"?: string;
  background?: LayerColor;
  bottom?: LayerLength;
  /** Centres the layer across its parent, whatever its width. */
  centred?: boolean;
  children?: ReactNode;
  /** The axis its children are laid along; a column left out. */
  direction?: "column" | "row";
  /** Covers the positioned parent edge to edge. */
  fill?: boolean;
  flex?: 1;
  height?: LayerLength;
  /** Takes its place in the flow, as a positioning context for the layers inside it. */
  inFlow?: boolean;
  left?: LayerLength;
  marginLeft?: number;
  minWidth?: number;
  /** A number, or a React Native Animated value or interpolation, followed without a re-render. */
  opacity?: Animated.Value | Animated.AnimatedInterpolation<number> | number;
  /** The web's key events from inside the layer; a native client moves focus by itself. */
  onKeyDown?: (event: {
    altKey: boolean;
    ctrlKey: boolean;
    key: string;
    metaKey: boolean;
    preventDefault: () => void;
  }) => void;
  onLayout?: (event: LayoutChangeEvent) => void;
  /** `none` lets touches and the pointer through. */
  pointerEvents?: "none";
  right?: LayerLength;
  role?: "group" | "img";
  testID?: string;
  top?: LayerLength;
  /** Fades the trailing edge out over this many points. Web only: a native client cuts at the edge. */
  trailingFade?: number;
  /** Fades the leading edge in over this many points. Web only: a native client cuts at the edge. */
  leadingFade?: number;
  width?: LayerLength;
  zIndex?: number;
};

const maskOf = (leading: number | undefined, trailing: number | undefined) => {
  const stops = [
    ...(leading === undefined ? [] : ["transparent", `black ${leading}px`]),
    trailing === undefined ? "black" : `black calc(100% - ${trailing}px)`,
    ...(trailing === undefined ? [] : ["transparent"]),
  ];
  return { maskImage: `linear-gradient(90deg, ${stops.join(", ")})` } as unknown as ViewStyle;
};

/**
 * A box placed inside a positioned parent, or (`inFlow`) the positioned parent itself. It draws
 * nothing but the background it is given, and sizes and places itself from its props alone. It is a
 * React Native View, so it keeps that View's position, stacking and shrink defaults.
 */
const Layer = ({
  background,
  bottom,
  centred = false,
  children,
  direction,
  fill = false,
  flex,
  height,
  inFlow = false,
  leadingFade,
  left,
  marginLeft,
  minWidth,
  onKeyDown,
  opacity,
  right,
  top,
  trailingFade,
  width,
  zIndex,
  ...host
}: LayerProps) => {
  const edge = fill ? 0 : undefined;
  return (
    <Animated.View
      {...host}
      // react-native-web hands a View's key events to the DOM; React Native's types leave it out.
      {...(Platform.OS === "web" && onKeyDown ? ({ onKeyDown } as object) : {})}
      style={[
        {
          backgroundColor: background && colors[background],
          bottom: bottom ?? edge,
          flex,
          flexDirection: direction,
          height,
          left: centred ? "50%" : (left ?? edge),
          marginLeft,
          minWidth,
          opacity,
          position: inFlow ? undefined : "absolute",
          right: right ?? edge,
          top: top ?? edge,
          transform: centred ? [{ translateX: "-50%" }] : undefined,
          width,
          zIndex,
        },
        (leadingFade !== undefined || trailingFade !== undefined) && Platform.OS === "web"
          ? maskOf(leadingFade, trailingFade)
          : null,
      ]}
    >
      {children}
    </Animated.View>
  );
};

export type { LayerColor, LayerProps };
export { Layer };
