import { Action, Badge, brandChroma, Surface, Text } from "@loomarr/design-system";

import type { DemoSignInProps } from "./demo-sign-in.type";

const withAlpha = (hex: string, alpha: number) => {
  const n = Number.parseInt(hex.slice(1), 16);
  return `rgba(${(n >> 16) & 255}, ${(n >> 8) & 255}, ${n & 255}, ${alpha})`;
};

// The same signal-tinted notice as DemoBanner, so the sign-in card and the tour read as one thing.
const SIGNAL = brandChroma[0];

// DemoSignIn is the demo site's way in (#2268): a visitor should not have to find a password to
// look around, so the sign-in screen offers the demo account as one button.
const DemoSignIn = ({ busy = false, onSignIn, username }: DemoSignInProps) => (
  <Surface
    aria-label="Loomarr demo"
    borderColor={withAlpha(SIGNAL, 0.35)}
    level="raised"
    overflow="hidden"
    role="region"
  >
    <Surface
      backgroundColor={withAlpha(SIGNAL, 0.07)}
      borderWidth={0}
      gap="inline"
      paddingHorizontal={16}
      paddingVertical={14}
    >
      <Surface
        alignItems="center"
        backgroundColor="transparent"
        borderWidth={0}
        flexDirection="row"
        gap="inline"
      >
        <Badge shape="square" tone="signal">
          Demo
        </Badge>
        <Text textRole="label" tone="primary">
          This is the Loomarr demo
        </Text>
      </Surface>
      <Text textRole="compact" tone="secondary">
        The channels and shows are invented, so look around as much as you like.
      </Text>
      <Action accessibilityRole="button" disabled={busy} icon="play" onPress={onSignIn}>
        {busy ? "Signing in…" : `Sign in as ${username}`}
      </Action>
    </Surface>
  </Surface>
);

export { DemoSignIn };
