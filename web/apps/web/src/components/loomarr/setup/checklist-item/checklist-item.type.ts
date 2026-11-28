type CheckStatus = "pending" | "running" | "pass" | "fail";

interface ChecklistItemProps {
  name: string;
  status: CheckStatus;
  hint?: string;
  docHref?: string;
}

export type { ChecklistItemProps, CheckStatus };
