import { channelNumber } from "@loomarr/core/format";
import { Badge, Card, Stack, Text } from "@loomarr/design-system";
import { NowNextStrip } from "../now-next-strip";
import { OnAirIndicator } from "../on-air-indicator";
import type { ChannelCardProps, ChannelHealth } from "./channel-card.type";

// ChannelCard — channel health at a glance (§3): the mono number, name, now/next,
// a "Managed by Loomarr" badge (§9 ownership), and a health chip. Composes the
// OnAir + NowNext atoms. States: healthy · pending-slots · drift · error · creating.
const HEALTH: Record<
  ChannelHealth,
  { label: string; tone: "caution" | "neutral" | "onair" | "tune" } | null
> = {
  healthy: null,
  "pending-slots": { label: "Filling in", tone: "tune" },
  drift: { label: "Drift", tone: "caution" },
  error: { label: "Error", tone: "onair" },
  creating: { label: "Creating", tone: "neutral" },
  // A deliberate operator pause — calm and neutral (a muted chip), never an error.
  paused: { label: "Paused", tone: "neutral" },
};

const ChannelCard = ({
  number,
  name,
  onAir,
  nowNext,
  managed = true,
  health = "healthy",
  trailing,
}: ChannelCardProps) => {
  const chip = HEALTH[health];
  return (
    <Card gap={12} padding={16}>
      <Stack align="start" direction="row" gap="control" justify="between">
        <Stack align="baseline" direction="row" gap="control">
          <Text as="span" fontVariant={["tabular-nums"]} textRole="channelNumber" tone="primary">
            {channelNumber(number)}
          </Text>
          <div>
            <Text as="p" textRole="cardLabel" tone="primary">
              {name}
            </Text>
            {managed && (
              <Text as="p" shout textRole="mono">
                Managed by Loomarr
              </Text>
            )}
          </div>
        </Stack>
        {/* Top-right cluster: the on-air dot, then any trailing action (the row ⋮ menu).
            Kept together so the action reads as part of the card, not adrift beside it. */}
        <Stack align="center" direction="row" flexShrink={0} gap="1.5">
          <OnAirIndicator state={onAir} />
          {trailing}
        </Stack>
      </Stack>

      <NowNextStrip {...nowNext} />

      {chip && (
        <Badge shape="square" tone={chip.tone}>
          {chip.label}
        </Badge>
      )}
    </Card>
  );
};

export { ChannelCard };
