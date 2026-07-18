import { Card } from "@/components/ui";
import { cn } from "@/lib";
import { NowNextStrip } from "../now-next-strip";
import { OnAirIndicator } from "../on-air-indicator";
import type { ChannelCardProps, ChannelHealth } from "./channel-card.type";

// ChannelCard — channel health at a glance (§3): the mono number, name, now/next,
// a "Managed by Loomarr" badge (§9 ownership), and a health chip. Composes the
// OnAir + NowNext atoms. States: healthy · pending-slots · drift · error · creating.
const HEALTH: Record<ChannelHealth, { label: string; chip: string } | null> = {
  healthy: null,
  "pending-slots": { label: "Backfilling", chip: "bg-tune-tint-15 text-tune" },
  drift: { label: "Drift", chip: "bg-caution-tint-15 text-caution" },
  error: { label: "Error", chip: "bg-onair-tint-15 text-onair-300" },
  creating: { label: "Creating", chip: "bg-static-800 text-static-400" },
};

const ChannelCard = ({
  number,
  name,
  onAir,
  nowNext,
  managed = true,
  health = "healthy",
  className,
}: ChannelCardProps) => {
  const chip = HEALTH[health];
  return (
    <Card className={cn("flex flex-col gap-3 p-4", className)}>
      <div className="flex items-start justify-between gap-3">
        <div className="flex items-baseline gap-3">
          <span className="font-mono text-2xl tabular-nums leading-none">{number}</span>
          <div>
            <p className="font-medium leading-tight">{name}</p>
            {managed && (
              <p className="font-mono text-[11px] text-static-400 uppercase tracking-wide">
                Managed by Loomarr
              </p>
            )}
          </div>
        </div>
        <OnAirIndicator state={onAir} />
      </div>

      <NowNextStrip {...nowNext} />

      {chip && (
        <span
          className={cn(
            "inline-flex w-fit items-center rounded-sm px-1.5 py-0.5 font-medium font-mono text-[11px] uppercase tracking-wide",
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
