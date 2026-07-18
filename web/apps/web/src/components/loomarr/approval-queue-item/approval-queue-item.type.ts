type ApprovalStatus = "pending" | "approving" | "denied";

interface ApprovalQueueItemProps {
  title: string;
  requestedBy?: string;
  summary?: string;
  acquisitions?: number;
  status?: ApprovalStatus;
  denyReason?: string;
  onApprove?: () => void;
  onDeny?: () => void;
  className?: string;
}

export type { ApprovalQueueItemProps, ApprovalStatus };
