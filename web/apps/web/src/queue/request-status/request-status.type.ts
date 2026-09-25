import type { ProposalItem } from "@loomarr/api/models/proposalItem";
import type { TitleDTO } from "@loomarr/api/models/titleDTO";

/** The three tabs of the Requests page; each is a path under `/requests`. */
type RequestTab = "needs-you" | "in-progress" | "done";

interface RequestStatus {
  tab: RequestTab;
  /** The single status line a card shows. */
  line: string;
  /** Badge variant for the line. */
  tone: "suggest" | "lock" | "onair" | "caution";
}

interface RequestAcquisition {
  item: ProposalItem;
  /** Its provisioning row; absent until the approval has enqueued it. */
  title?: TitleDTO;
}

export type { RequestAcquisition, RequestStatus, RequestTab };
