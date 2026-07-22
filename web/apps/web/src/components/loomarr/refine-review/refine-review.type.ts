import type { ProposalItem } from "@loomarr/api";

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
  onApply: () => void;
  onDiscard: () => void;
  busy?: boolean;
  className?: string;
}

export type { CurrentLineupItem, RefineReviewProps };
