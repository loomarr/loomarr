import { View } from "@tamagui/core";

import type { VisuallyHiddenProps } from "./visually-hidden.type";

// The native adapter: a one-point box clipped to nothing. A screen reader still walks into it and
// reads the children, which are the caller's own `Text`.
const VisuallyHidden = ({ children }: VisuallyHiddenProps) => (
  <View height={1} margin={-1} overflow="hidden" position="absolute" width={1}>
    {children}
  </View>
);

export { VisuallyHidden };
