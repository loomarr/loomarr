import type { NowNext } from "../now-next-strip";
import type { OnAirState } from "../on-air-indicator";

type ChannelHealth = "healthy" | "pending-slots" | "drift" | "error" | "creating";

interface ChannelCardProps {
  number: number;
  name: string;
  onAir: OnAirState;
  nowNext?: NowNext;
  managed?: boolean;
  health?: ChannelHealth;
  className?: string;
}

export type { ChannelCardProps, ChannelHealth };
