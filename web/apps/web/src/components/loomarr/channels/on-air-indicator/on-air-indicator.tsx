import { Stack, StatusDot, Text } from "@loomarr/design-system";
import type { OnAirIndicatorProps, OnAirState } from "./on-air-indicator.type";

// OnAirIndicator — the red dot (§3). `live` pulses (the one live-motion signature,
// §2.4) and is still under reduced-motion; `reconciling` is a steady caution dot;
// `off` is a muted dot. The dot is labelled, or the visible label says it, so screen readers
// say "On air", not a bare glyph (§5.3).
const TONE = {
  off: "off",
  live: "live",
  reconciling: "pending",
} as const satisfies Record<OnAirState, string>;

const SR: Record<OnAirState, string> = {
  off: "Off air",
  live: "On air",
  reconciling: "Updating",
};

const OnAirIndicator = ({ state, showLabel = false, className }: OnAirIndicatorProps) => (
  <Stack align="center" className={className} direction="row" gap="1.5" render="span">
    <StatusDot label={showLabel ? "" : SR[state]} tone={TONE[state]} />
    {showLabel && (
      <Text as="span" textRole="guideCell" tone={state === "live" ? "live" : "secondary"}>
        {SR[state]}
      </Text>
    )}
  </Stack>
);

export { OnAirIndicator };
