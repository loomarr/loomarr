import type { SessionBody } from "@loomarr/api/models/sessionBody";
import type { UserBody } from "@loomarr/api/models/userBody";

interface PersonDetailProps {
  user: UserBody;
  isSelf?: boolean;
  busy?: boolean;
  sessions: SessionBody[];
  sessionsLoading?: boolean;
  revoking?: string;
  onRoleChange?: (role: "admin" | "member") => void;
  onQuotaChange?: (quota: number) => void;
  onToggleDisabled?: (disabled: boolean) => void;
  onToggleAutoApprove?: (autoApprove: boolean) => void;
  onRevokeSession?: (id: string) => void;
  onResetPassword?: (next: string) => Promise<void>;
}

export type { PersonDetailProps };
