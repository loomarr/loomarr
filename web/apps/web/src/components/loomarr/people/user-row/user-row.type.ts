import type { UserBody } from "@loomarr/api/models/userBody";

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
  // Reset this user's password (admin path — no current password required). Offered
  // ONLY for a local account: an imported user's credential lives on the media server
  // and Loomarr never held it. The row's local/media-server label has always existed
  // to explain "whether a password reset is even meaningful" — this is the action it
  // was explaining.
  onResetPassword?: () => void;
  className?: string;
}

export type { UserRowProps };
