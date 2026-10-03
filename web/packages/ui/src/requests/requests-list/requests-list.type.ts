import type { ProposalDTO, RequestEntry, RequestsSnapshot, RequestTab } from "@loomarr/core/requests";

type RequestsListProps = {
  onApprove: (proposal: ProposalDTO) => void;
  onApproveSelected: (proposalIds: readonly string[]) => void;
  onDeny: (proposal: ProposalDTO, reason?: string) => void;
  /** Edit and try again: resumes the failed request in the form. */
  onFix: (entry: RequestEntry) => void;
  onOpen: (jobId: string) => void;
  onOpenChannel: (channelId: string) => void;
  onRequestChannel: () => void;
  onRetry: () => void;
  onTabChange: (tab: RequestTab) => void;
  snapshot: RequestsSnapshot;
  tab: RequestTab;
};

export type { RequestsListProps };
