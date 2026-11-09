import { Action, StatusDot, Surface, Text } from "@loomarr/design-system";
import { Fragment, type ReactNode } from "react";

import type { CardActionProps, CardStatusProps, ChannelChipProps } from "./card-parts.type";

// The small pieces both card families share (#2765): a status (a dot and a word), the channel chip,
// the one metadata line and the one action.

// A dot and a word. The dot sits beside text that already says it, so it carries no label of its own.
const CardStatus = ({ label, tone }: CardStatusProps) => (
  <Surface alignItems="center" backgroundColor="transparent" borderWidth={0} flexDirection="row" gap={6}>
    <StatusDot label="" tone={tone} />
    <Text textRole="cardMeta" tone="secondary">
      {label}
    </Text>
  </Surface>
);

// "03 Late Night Sci-Fi": the channel number, then its name, on a neutral chip.
const ChannelChip = ({ name, number }: ChannelChipProps) => (
  <Surface
    alignItems="center"
    backgroundColor="surfaceElevated"
    borderRadius={4}
    borderWidth={0}
    flexDirection="row"
    gap={6}
    maxWidth="100%"
    paddingHorizontal={6}
    paddingVertical={2}
  >
    <Text textRole="cardChannelNumber">{number}</Text>
    <Text flexShrink={1} numberOfLines={1} textRole="cardMeta" tone="secondary">
      {name}
    </Text>
  </Surface>
);

// The one metadata line: parts joined by a middle dot, wrapping on a narrow screen.
const CardMeta = ({ parts }: { parts: ReactNode[] }) => (
  <Surface
    alignItems="center"
    backgroundColor="transparent"
    borderWidth={0}
    columnGap={6}
    flexDirection="row"
    flexWrap="wrap"
    rowGap={2}
  >
    {parts.map((part, index) => (
      // The parts are a fixed, ordered line, never reordered.
      // biome-ignore lint/suspicious/noArrayIndexKey: positional by design
      <Fragment key={index}>
        {index > 0 && (
          <Text aria-hidden textRole="cardMeta" tone="secondary">
            ·
          </Text>
        )}
        {typeof part === "string" ? (
          <Text textRole="cardMeta" tone="secondary">
            {part}
          </Text>
        ) : (
          part
        )}
      </Fragment>
    ))}
  </Surface>
);

// The one action at the trailing edge: quiet at rest, a secondary button when the item needs attention.
const CardAction = ({ attention = false, ...props }: CardActionProps) => (
  <Surface backgroundColor="transparent" borderWidth={0} flexShrink={0}>
    <Action variant={attention ? "outline" : "ghost"} {...props} />
  </Surface>
);

export { CardAction, CardMeta, CardStatus, ChannelChip };
