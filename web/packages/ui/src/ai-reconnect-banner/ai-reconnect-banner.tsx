import { Action, brandChroma, Surface, Text } from "@loomarr/design-system";

import type { AiReconnectBannerProps } from "./ai-reconnect-banner.type";

const withAlpha = (hex: string, alpha: number) => {
  const n = Number.parseInt(hex.slice(1), 16);
  return `rgba(${(n >> 16) & 255}, ${(n >> 8) & 255}, ${n & 255}, ${alpha})`;
};

// The mock's `.notice.suggest-tint`: a suggest-hued border over a faint suggest wash on the raised
// surface. The wash is its own layer so the raised colour stays the theme's.
const SUGGEST = brandChroma[4];

// AiReconnectBanner is the manual builder's resume banner (#1817, approved mock #1872 Alt A): AI
// came back while someone was picking titles by hand. It offers the AI path without taking the
// hand-picked lineup away — the copy says so, and the caller is what keeps that promise.
const AiReconnectBanner = ({
  busy = false,
  onKeepManual,
  onTryAi,
  pickCount,
  tryAiBlockedReason,
}: AiReconnectBannerProps) => (
  <Surface
    aria-live="polite"
    borderColor={withAlpha(SUGGEST, 0.35)}
    level="raised"
    overflow="hidden"
    role="status"
  >
    <Surface
      backgroundColor={withAlpha(SUGGEST, 0.07)}
      borderWidth={0}
      gap="inline"
      paddingHorizontal={16}
      paddingVertical={14}
    >
      <Text textRole="label" tone="primary">
        AI is connected now
      </Text>
      <Text textRole="compact" tone="secondary">
        {`Your manual lineup (${pickCount} ${pickCount === 1 ? "title" : "titles"}) is still here. Keep building it by hand, or let Loomarr suggest titles for this description — your picks stay in the lineup either way.`}
      </Text>
      <Surface backgroundColor="transparent" borderWidth={0} flexDirection="row" flexWrap="wrap" gap="inline">
        <Action
          accessibilityRole="button"
          disabled={busy || tryAiBlockedReason !== undefined}
          onPress={onTryAi}
          variant="outline"
        >
          Try AI suggestions
        </Action>
        <Action accessibilityRole="button" disabled={busy} onPress={onKeepManual} variant="ghost">
          Keep building manually
        </Action>
      </Surface>
      {tryAiBlockedReason ? (
        <Text textRole="caption" tone="secondary">
          {tryAiBlockedReason}
        </Text>
      ) : null}
    </Surface>
  </Surface>
);

export { AiReconnectBanner };
