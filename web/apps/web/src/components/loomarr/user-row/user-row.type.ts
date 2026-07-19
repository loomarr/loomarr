import type { UserBody } from "@loomarr/api";

interface UserRowProps {
  user: UserBody;
  // True while a change to THIS user is in flight, so the row's controls disable
  // without freezing the whole table.
  busy?: boolean;
  // Set when this row is the signed-in admin. Demoting or disabling yourself is the
  // one destructive action with no undo path from inside the app, so the row refuses
  // it rather than relying on the operator to notice.
  isSelf?: boolean;
  onRoleChange?: (role: "admin" | "member") => void;
  onQuotaChange?: (quota: number) => void;
  onToggleDisabled?: (disabled: boolean) => void;
  onToggleAutoApprove?: (autoApprove: boolean) => void;
  // Opens this user's session list. Absent ⇒ the control is hidden.
  onViewSessions?: () => void;
  className?: string;
}

export type { UserRowProps };
