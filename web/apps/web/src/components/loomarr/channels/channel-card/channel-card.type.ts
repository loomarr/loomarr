import type { ReactNode } from "react";
import type { NowNext } from "../now-next-strip";
import type { OnAirState } from "../on-air-indicator";

// ChannelHealth is a FE-derived *rollup*, NOT a mirror of the generated `channelDTOStatus`
// (building/live/drifted/detached): a page maps the API status + slot-fill + error signals
// into this presentational health. It has no 1:1 API enum, so it stays hand-written (§4.2).
type ChannelHealth = "healthy" | "pending-slots" | "drift" | "error" | "creating" | "paused";

interface ChannelCardProps {
  number: number;
  name: string;
  onAir: OnAirState;
  nowNext?: NowNext;
  managed?: boolean;
  health?: ChannelHealth;
  // An optional action rendered in the top-right cluster, to the RIGHT of the on-air dot
  // (e.g. a row's ⋮ menu). Lives inside the card so a per-row action reads as part of the
  // row it acts on, rather than floating in a margin beside it. Omitted → just the dot.
  trailing?: ReactNode;
  className?: string;
}

export type { ChannelCardProps, ChannelHealth };
