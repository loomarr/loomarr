import type { ChannelPolicy } from "@loomarr/api/models/channelPolicy";
import type { CurrentLineupItem } from "../refine-review";

interface RefinePanelProps {
  channelId: string;
  channelName: string;
  // The channel's current lineup, for RefineReview's diff — see its prop doc. Passed
  // through unchanged; RefinePanel doesn't interpret it itself.
  current?: CurrentLineupItem[];
  // The channel's current policy, for RefineReview's programming-delta chips (§8.2).
  currentPolicy?: ChannelPolicy;
  // Fires after an apply lands (approve succeeds) — the caller's cue to refetch the
  // channel so the page reflects the patched lineup.
  onApplied?: () => void;
  className?: string;
}

export type { RefinePanelProps };
