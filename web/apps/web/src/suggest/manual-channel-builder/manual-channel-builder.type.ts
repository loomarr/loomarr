import type { ChannelIdeaDTO } from "@loomarr/api/models/channelIdeaDTO";

interface ManualChannelBuilderProps {
  // Admin sees "Add a channel" / "Create channel"; member sees "Request a channel" /
  // "Send for approval" — the SAME copy the existing describe panel already uses (§12).
  isAdmin: boolean;
  // Seeds the builder from an idea card's "Edit first" (#1817): the idea's name, pitch and
  // already-grounded titles land straight in the lineup, ready to edit before continuing.
  // Templates and facet chips are hidden in this mode — the idea already is the starting point.
  initialIdea?: ChannelIdeaDTO;
  onCancel: () => void;
  // The new proposal's job id, once SubmitBuilt has recorded it — hand this straight to
  // useSuggestionRun/ChannelSuggestPanel so review renders through the EXISTING
  // submit → review → approve/deny gate, unchanged.
  onSubmitted: (jobId: string) => void;
  className?: string;
}

export type { ManualChannelBuilderProps };
