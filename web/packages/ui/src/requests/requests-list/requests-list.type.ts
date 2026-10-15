import type {
  ProposalDTO,
  PullDTO,
  RequestEntry,
  RequestsSnapshot,
  RequestTab,
} from "@loomarr/core/requests";

import type { RequestChannelName } from "../request-done-row";
import type { ReviewAction, ReviewState } from "../review";

type RequestsListProps = {
  /** The guide's channels, so a Done row can name its channel by number and name. */
  channels?: readonly RequestChannelName[];
  /** Wall-clock now for relative dates; fixed in stories and tests. */
  nowMs?: number;
  onApprove: (proposal: ProposalDTO) => void;
  onApproveFiller: (pull: PullDTO) => void;
  /** Edit and try again: resumes the failed request in the form. */
  onFix: (entry: RequestEntry) => void;
  onOpen: (jobId: string) => void;
  onOpenChannel: (channelId: string) => void;
  onRequestChannel: () => void;
  onRetry: () => void;
  /** Select, tick, Deny, Dismiss and Edit: the host owns the review state and its one docked sheet. */
  onReview: (action: ReviewAction) => void;
  onTabChange: (tab: RequestTab) => void;
  review: ReviewState;
  snapshot: RequestsSnapshot;
  tab: RequestTab;
};

export type { RequestsListProps };
