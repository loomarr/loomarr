import { createContext, type PropsWithChildren, useContext } from "react";

import { type Density, semanticSpace } from "../tokens";

type ViewportInsets = {
  bottom: number;
  left: number;
  right: number;
  top: number;
};

const emptyViewportInsets: ViewportInsets = { bottom: 0, left: 0, right: 0, top: 0 };

/** Android TV's established safe-title margin. It is platform policy, not a spacing token. */
const TV_OVERSCAN_INSET = 48;

const densityGutter: Record<Density, number> = {
  pointer: semanticSpace.screen,
  touch: semanticSpace.section,
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

const ViewportInsetsProvider = ({
  children,
  insets = emptyViewportInsets,
}: PropsWithChildren<{ insets?: ViewportInsets }>) => (
  <ViewportInsetsContext.Provider value={insets}>{children}</ViewportInsetsContext.Provider>
);

const useViewportInsets = (): ViewportInsets => useContext(ViewportInsetsContext);

/** Resolve platform safe-area values and Loomarr's density gutter at the point of use. */
const useResolvedViewportInsets = (density: Density): ViewportInsets =>
  resolveViewportInsets(density, useViewportInsets());

export type { ViewportInsets };
export {
  emptyViewportInsets,
  resolveViewportInsets,
  TV_OVERSCAN_INSET,
  useResolvedViewportInsets,
  useViewportInsets,
  ViewportInsetsProvider,
};
