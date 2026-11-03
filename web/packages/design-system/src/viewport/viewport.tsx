import { createContext, type PropsWithChildren, useContext, useMemo } from "react";
import { Platform, useWindowDimensions, View } from "react-native";

import { type Density, semanticSpace } from "../tokens";

/**
 * The width a TV screen lays out in. Android TV reports a 1080p screen as 960×540 dp at 2× density,
 * and every tv-density size is drawn for that. tvOS reports the same screen as 1920×1080 points at
 * 1×, so on Apple TV the stage lays the app out at this width and scales it up to the screen. The
 * tvOS render-scale config plugin has the scaled layers rasterize at the screen's resolution
 * rather than upscale a half-size bitmap.
 */
const TV_LAYOUT_WIDTH = 960;

const isAppleTv = Platform.OS === "ios" && Platform.isTV;

/** How many screen points one layout unit takes: above 1 only on Apple TV. */
const resolveViewportScale = (windowWidth: number, appleTv = isAppleTv): number =>
  appleTv ? windowWidth / TV_LAYOUT_WIDTH : 1;

const useViewportScale = (): number => resolveViewportScale(useWindowDimensions().width);

/**
 * The root of everything drawn: the app's, and each native modal's, since a modal is a native root
 * of its own. On Apple TV it lays its children out in TV layout units and scales them to the
 * screen; everywhere else it renders them as they are.
 */
const ViewportStage = ({ children }: PropsWithChildren) => {
  const { height, width } = useWindowDimensions();
  const scale = resolveViewportScale(width);
  if (scale === 1) return <>{children}</>;
  return (
    <View
      style={{
        height: height / scale,
        transform: [{ scale }],
        transformOrigin: "top left",
        width: width / scale,
      }}
    >
      {children}
    </View>
  );
};

type ViewportInsets = {
  bottom: number;
  left: number;
  right: number;
  top: number;
};

const emptyViewportInsets: ViewportInsets = { bottom: 0, left: 0, right: 0, top: 0 };

/** Android TV's established safe-title margin. It is platform policy, not a spacing token. */
const TV_OVERSCAN_INSET = 48;

/** A phone screen's side gutter: every iPhone and Android frame of the native mock (5d–5g) sets 16. */
const PHONE_GUTTER = 16;

const densityGutter: Record<Density, number> = {
  pointer: semanticSpace.screen,
  touch: PHONE_GUTTER,
  tv: TV_OVERSCAN_INSET,
};

const resolveViewportInsets = (
  density: Density,
  platformInsets: ViewportInsets = emptyViewportInsets,
): ViewportInsets => {
  const gutter = densityGutter[density];
  return {
    bottom: gutter + platformInsets.bottom,
    left: gutter + platformInsets.left,
    right: gutter + platformInsets.right,
    top: gutter + platformInsets.top,
  };
};

const ViewportInsetsContext = createContext<ViewportInsets>(emptyViewportInsets);

/** Platform safe-area insets arrive in screen points; the stage lays its children out in layout units. */
const ViewportInsetsProvider = ({
  children,
  insets = emptyViewportInsets,
}: PropsWithChildren<{ insets?: ViewportInsets }>) => {
  const scale = useViewportScale();
  const value = useMemo(
    () =>
      scale === 1
        ? insets
        : {
            bottom: insets.bottom / scale,
            left: insets.left / scale,
            right: insets.right / scale,
            top: insets.top / scale,
          },
    [insets, scale],
  );
  return <ViewportInsetsContext.Provider value={value}>{children}</ViewportInsetsContext.Provider>;
};

const useViewportInsets = (): ViewportInsets => useContext(ViewportInsetsContext);

/** Resolve platform safe-area values and Loomarr's density gutter at the point of use. */
const useResolvedViewportInsets = (density: Density): ViewportInsets =>
  resolveViewportInsets(density, useViewportInsets());

export type { ViewportInsets };
export {
  emptyViewportInsets,
  PHONE_GUTTER,
  resolveViewportInsets,
  resolveViewportScale,
  TV_OVERSCAN_INSET,
  useResolvedViewportInsets,
  useViewportInsets,
  useViewportScale,
  ViewportInsetsProvider,
  ViewportStage,
};
