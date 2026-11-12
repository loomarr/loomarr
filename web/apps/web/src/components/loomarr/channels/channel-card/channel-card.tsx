import { channelNumber } from "@loomarr/core/format";
import { Card, Stack, Text } from "@loomarr/design-system";
import { cn } from "@/lib/utils";
import { NowNextStrip } from "../now-next-strip";
import { OnAirIndicator } from "../on-air-indicator";
import type { ChannelCardProps, ChannelHealth } from "./channel-card.type";

// ChannelCard — channel health at a glance (§3): the mono number, name, now/next,
// a "Managed by Loomarr" badge (§9 ownership), and a health chip. Composes the
// OnAir + NowNext atoms. States: healthy · pending-slots · drift · error · creating.
const HEALTH: Record<
  ChannelHealth,
  { label: string; chip: string; tone: "danger" | "info" | "secondary" | "warning" } | null
> = {
  healthy: null,
  "pending-slots": { label: "Filling in", chip: "bg-tune-tint-15", tone: "info" },
  drift: { label: "Drift", chip: "bg-caution-tint-15", tone: "warning" },
  error: { label: "Error", chip: "bg-onair-tint-15", tone: "danger" },
  creating: { label: "Creating", chip: "bg-static-800", tone: "secondary" },
  // A deliberate operator pause — calm and neutral (a muted chip), never an error.
  paused: { label: "Paused", chip: "bg-static-800", tone: "secondary" },
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
          <span className="font-mono text-2xl leading-none tabular-nums">{channelNumber(number)}</span>
          <div>
            <p className="leading-tight font-medium">{name}</p>
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
        <Text
          as="span"
          className={cn("inline-flex w-fit items-center rounded-sm px-1.5 py-0.5", chip.chip)}
          textRole="mono"
          tone={chip.tone}
        >
          {chip.label}
        </Text>
      )}
    </Card>
  );
};

export { ChannelCard };
