import { Action, Badge, brandChroma, Surface, Text } from "@loomarr/design-system";

import type { DemoBannerProps } from "./demo-banner.type";

const withAlpha = (hex: string, alpha: number) => {
  const n = Number.parseInt(hex.slice(1), 16);
  return `rgba(${(n >> 16) & 255}, ${(n >> 8) & 255}, ${n & 255}, ${alpha})`;
};

// The same notice as AiReconnectBanner (the mock's `.notice` tint), in signal amber: the demo is
// the one thing on every page that is about Loomarr itself rather than the household's TV.
const SIGNAL = brandChroma[0];

// DemoBanner tells a visitor they are on the demo site and offers the next step of a short tour
// (#2268). It sits at the top of every page while the server says it is a demo site.
//
// `compact` is the phone layout (#2750). The wide layout wraps into a message row, a next-step row
// and a row for the X, about 100px of an 844px screen. Compact keeps one line under 56px: the Demo
// badge already says what the sentence said, so the line is the badge, the next step and the X.
const DemoBanner = ({ compact = false, step, stepCount, next, onDismiss }: DemoBannerProps) => (
  <Surface
    aria-label="Demo tour"
    borderColor={withAlpha(SIGNAL, 0.35)}
    borderLeftWidth={0}
    borderRightWidth={0}
    borderTopWidth={0}
    borderRadius={0}
    level="raised"
    overflow="hidden"
    role="region"
  >
    <Surface
      alignItems="center"
      backgroundColor={withAlpha(SIGNAL, 0.07)}
      borderWidth={0}
      borderRadius={0}
      flexDirection="row"
      flexWrap={compact ? "nowrap" : "wrap"}
      gap="inline"
      paddingHorizontal={16}
      paddingVertical={compact ? 4 : 8}
    >
      {/* Its own row: a square Badge aligns itself to the top, so beside the 40px actions it would
          ride above the message instead of sitting level with it. */}
      <Surface
        alignItems="center"
        backgroundColor="transparent"
        borderWidth={0}
        flexDirection="row"
        flexGrow={1}
        flexShrink={1}
        gap="inline"
        minWidth={0}
      >
        <Badge shape="square" tone="signal">
          Demo
        </Badge>
        {compact && next ? (
          <>
            <Text textRole="compact" tone="secondary">
              Next:
            </Text>
            <Action accessibilityRole="link" render={next.link} variant="link">
              {next.label}
            </Action>
          </>
        ) : (
          <Text flexShrink={1} numberOfLines={compact ? 1 : undefined} textRole="compact" tone="primary">
            {next
              ? "You're exploring the Loomarr demo."
              : compact
                ? "That's the tour."
                : "That's the tour. It's a demo, so explore freely."}
          </Text>
        )}
      </Surface>
      {next && !compact ? (
        <Surface
          alignItems="center"
          backgroundColor="transparent"
          borderWidth={0}
          flexDirection="row"
          gap="inline"
        >
          <Text textRole="compact" tone="secondary">
            Next:
          </Text>
          {/* `link`, or Action stamps `role="button"` on the anchor and it stops reading as a link. */}
          <Action accessibilityRole="link" render={next.link} variant="link">
            {next.label}
          </Action>
          <Text textRole="caption" tone="secondary">
            {`Step ${step} of ${stepCount}`}
          </Text>
        </Surface>
      ) : null}
      <Action accessibilityLabel="Hide the demo tour" icon="close" onPress={onDismiss} variant="ghost">
        {null}
      </Action>
    </Surface>
  </Surface>
);

export { DemoBanner };
