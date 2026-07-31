import { channelNumber } from "@loomarr/core";
import { Caption, Card } from "@/components/ui";
import { cn } from "@/lib";
import { NowNextStrip } from "../now-next-strip";
import { OnAirIndicator } from "../on-air-indicator";
import type { ChannelCardProps, ChannelHealth } from "./channel-card.type";

// ChannelCard — channel health at a glance (§3): the mono number, name, now/next,
// a "Managed by Loomarr" badge (§9 ownership), and a health chip. Composes the
// OnAir + NowNext atoms. States: healthy · pending-slots · drift · error · creating.
const HEALTH: Record<ChannelHealth, { label: string; chip: string } | null> = {
  healthy: null,
  "pending-slots": { label: "Filling in", chip: "bg-tune-tint-15 text-tune" },
  drift: { label: "Drift", chip: "bg-caution-tint-15 text-caution" },
  error: { label: "Error", chip: "bg-onair-tint-15 text-onair-300" },
  creating: { label: "Creating", chip: "bg-static-800 text-static-400" },
  // A deliberate operator pause — calm and neutral (a muted chip), never an error.
  paused: { label: "Paused", chip: "bg-static-800 text-static-400" },
};

const ChannelCard = ({
  number,
  name,
  onAir,
  nowNext,
  managed = true,
  health = "healthy",
  trailing,
  className,
}: ChannelCardProps) => {
  const chip = HEALTH[health];
  return (
    <Card className={cn("flex flex-col gap-3 p-4", className)}>
      <div className="flex items-start justify-between gap-3">
        <div className="flex items-baseline gap-3">
          <span className="font-mono text-2xl tabular-nums leading-none">{channelNumber(number)}</span>
          <div>
            <p className="font-medium leading-tight">{name}</p>
            {managed && (
              <Caption as="p" shout>
                Managed by Loomarr
              </Caption>
            )}
          </div>
        </div>
        {/* Top-right cluster: the on-air dot, then any trailing action (the row ⋮ menu).
            Kept together so the action reads as part of the card, not adrift beside it. */}
        <div className="flex shrink-0 items-center gap-1.5">
          <OnAirIndicator state={onAir} />
          {trailing}
        </div>
      </div>

      <NowNextStrip {...nowNext} />

      {chip && (
        <span
          className={cn(
            "inline-flex w-fit items-center rounded-sm px-1.5 py-0.5 font-medium font-mono text-2xs uppercase tracking-wide",
            chip.chip,
          )}
        >
          {chip.label}
        </span>
      )}
    </Card>
  );
};

export { ChannelCard };
