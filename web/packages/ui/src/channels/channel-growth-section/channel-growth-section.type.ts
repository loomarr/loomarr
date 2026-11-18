/** Why a channel isn't growing; empty when it is. */
type GrowthStill = "" | "kept" | "by_hand" | "fixed" | "not_active";

/** A count of what a channel holds or lacks, by kind. */
interface GrowthCounts {
  episodes: number;
  shows: number;
  films: number;
}

/** A title that joined the channel by itself, on the night recorded in `batch` (growth-YYYY-MM-DD). */
interface GrowthJoinedTitle {
  batch: string;
  key: string;
  name: string;
}

/** A title taken back off: `reason` is the model's when it took it off, empty when a person did. */
interface GrowthOffTitle {
  key: string;
  name: string;
  reason: string;
}

interface ChannelGrowthSectionProps {
  /** Disables every control while a change is saving. */
  busy?: boolean;
  /** The "Keep as it is" switch. */
  keep: boolean;
  /** The line, in plain words (`line.words`). */
  lineWords: string;
  joined: readonly GrowthJoinedTitle[];
  off: readonly GrowthOffTitle[];
  onAddBack: (key: string) => void;
  onKeepChange: (keep: boolean) => void;
  onTakeOff: (key: string) => void;
  short: GrowthCounts;
  still: GrowthStill;
  target: GrowthCounts;
  /** Unclear fits waiting for the model's verdict. */
  waiting: number;
}

export type { ChannelGrowthSectionProps, GrowthCounts, GrowthJoinedTitle, GrowthOffTitle, GrowthStill };
