import type { ReactNode } from "react";
import { View } from "react-native";

import { StatusDot, type StatusTone, Text } from "../primitives";

type StatusLabelProps = {
  /** The sentence-case word: "Enabled", "Admin", "Set via environment". */
  children: ReactNode;
  tone?: StatusTone;
};

/**
 * Quiet status: a small dot and a sentence-case word, with no fill. Where a filled chip shouts
 * (ADMIN, ENABLED, DEFAULT), this reads as part of the row. The word carries the meaning; the dot
 * only tints it, so it is hidden from assistive tech.
 */
const StatusLabel = ({ children, tone = "off" }: StatusLabelProps) => (
  <View style={{ alignItems: "center", alignSelf: "flex-start", flexDirection: "row", gap: 6 }}>
    <StatusDot label="" size="compact" tone={tone} />
    <Text textRole="compact" tone="secondary">
      {children}
    </Text>
  </View>
);

export type { StatusLabelProps };
export { StatusLabel };
