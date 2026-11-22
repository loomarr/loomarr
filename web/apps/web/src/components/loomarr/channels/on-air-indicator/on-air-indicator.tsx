import { Stack, Text } from "@loomarr/design-system";
import type { OnAirIndicatorProps, OnAirState } from "./on-air-indicator.type";

// OnAirIndicator — the red dot (§3). `live` pulses (the one live-motion signature,
// §2.4) and is off under reduced-motion; `reconciling` is a steady caution dot;
// `off` is a muted dot. A sr-only sentence makes screen readers say "On air", not
// a bare glyph (§5.3).
const DOT = {
  off: "$actionDisabled",
  live: "$guideOnAir",
  reconciling: "$stateWarning",
} as const satisfies Record<OnAirState, string>;

const SR: Record<OnAirState, string> = {
  off: "Off air",
  live: "On air",
  reconciling: "Updating",
};

const OnAirIndicator = ({ state, showLabel = false, className }: OnAirIndicatorProps) => (
  <Stack align="center" className={className} direction="row" gap="1.5" render="span">
    <Stack height={10} position="relative" render="span" width={10}>
      {state === "live" && (
        <Stack
          aria-hidden
          backgroundColor="$guideOnAir"
          borderRadius={999}
          className="motion-safe:animate-ping"
          height="100%"
          opacity={0.6}
          position="absolute"
          render="span"
          width="100%"
        />
      )}
      <Stack
        aria-hidden
        backgroundColor={DOT[state]}
        borderRadius={999}
        height={10}
        position="relative"
        render="span"
        width={10}
      />
    </Stack>
    {showLabel && (
      <Text as="span" textRole="guideCell" tone={state === "live" ? "live" : "secondary"}>
        {SR[state]}
      </Text>
    )}
    <span className="sr-only">{SR[state]}</span>
  </Stack>
);

export { OnAirIndicator };
