import type { ProposalItem } from "@loomarr/api/models/proposalItem";
import type { TitleDTO } from "@loomarr/api/models/titleDTO";

/** The three tabs of the Requests page; each is a path under `/requests`. */
type RequestTab = "needs-you" | "in-progress" | "done";

interface RequestStatus {
  tab: RequestTab;
  /** The SHORT badge label — never a sentence; the sentence is `detail`. */
  line: string;
  /** The one-sentence explanation shown beside the badge, when the label alone is not enough. */
  detail?: string;
  /** Badge variant for the line. */
  tone: "suggest" | "lock" | "onair" | "caution";
}

interface RequestAcquisition {
  item: ProposalItem;
  /** Its provisioning row; absent until the approval has enqueued it. */
  title?: TitleDTO;
}

export type { RequestAcquisition, RequestStatus, RequestTab };
