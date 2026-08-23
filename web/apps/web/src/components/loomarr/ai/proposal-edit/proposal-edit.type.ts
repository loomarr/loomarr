import type { ApprovalEditDTO } from "@loomarr/api/models/approvalEditDTO";
import type { ProposalItem } from "@loomarr/api/models/proposalItem";

interface ProposalEditProps {
  // The proposal's two pick lists, as the queue already renders them. Both are needed:
  // dropping an in-library title changes the channel, dropping an acquisition also stops a
  // download, and the approver should see one list of "what I am approving".
  lineup: ProposalItem[];
  acquisitions: ProposalItem[];
  // Called on every change with the edit as the API wants it — or `undefined` when nothing has
  // been modified. Undefined is not the same as an empty edit (see the component): the caller
  // must send NO body in that case, so an unmodified approval stays byte-identical to what it
  // was before edit-before-approve existed.
  onChange: (edit: ApprovalEditDTO | undefined) => void;
  onFeedback?: (item: ProposalItem, action: "keep" | "less" | "never" | "surprise") => void;
  disabled?: boolean;
  className?: string;
}

export type { ProposalEditProps };
