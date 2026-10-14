import type { ChannelPolicy } from "@loomarr/api/models/channelPolicy";
import type { ProposalItem } from "@loomarr/api/models/proposalItem";

// A row from the channel's current lineup (ChannelDTO.lineup / LineupEntryDTO) — only
// the fields the diff needs. `key` is the provisioning key ("movie:tmdb:603",
// "series:tvdb:71663", §3); when it's missing (older callers/tests) the diff falls back
// to matching by name+year.
interface CurrentLineupItem {
  name: string;
  year?: number;
  key?: string;
}

interface RefineReviewProps {
  proposed: ProposalItem[];
  acquisitions?: ProposalItem[];
  // The channel's current lineup, for the kept/added/removed diff. `ch.lineup` from
  // ChannelDTO (§7 refine) — optional so the component stays testable/storyable without
  // it, but a real page always has it.
  current?: CurrentLineupItem[];
  // The channel's current policy + the refined proposal's policy, for the programming-
  // delta chips (§8.2): a refine can change scope/audience/ordering/seasonal, and those
  // changes used to apply invisibly. Showing them — and marking a field the operator
  // pinned as "kept" (operator-dirty stickiness) — is the visible half of that fix. Both
  // optional so the component stays storyable without them.
  currentPolicy?: ChannelPolicy;
  proposedPolicy?: ChannelPolicy;
  onApply: () => void;
  onDiscard: () => void;
  busy?: boolean;
  className?: string;
}

export type { CurrentLineupItem, RefineReviewProps };
