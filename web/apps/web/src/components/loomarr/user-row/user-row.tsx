import { KeyRound, MonitorSmartphone, Server } from "lucide-react";
import { Badge, Button, Checkbox, Input, Label, Select } from "@/components/ui";
import { cn } from "@/lib";
import type { UserRowProps } from "./user-row.type";

// UserRow — one row of the §11 allowlist. Role, quota, auto-approve, and disabled are
// Loomarr-owned regardless of how the user authenticates, so they are all editable here;
// the credential path is shown but never editable, because it is a consequence of how the
// account was created (local bootstrap vs. media-server import), not a setting.
//
// Every control writes immediately. There is no save bar: each field is an independent
// PATCH, and batching them would invite the "I toggled disabled and forgot to save"
// failure on the one screen where that means someone keeps their access.
const UserRow = ({
  user,
  busy,
  isSelf,
  onRoleChange,
  onQuotaChange,
  onToggleDisabled,
  onToggleAutoApprove,
  onViewSessions,
  className,
}: UserRowProps) => {
  const roleId = `role-${user.id}`;
  const quotaId = `quota-${user.id}`;
  const autoId = `auto-${user.id}`;

  return (
    <div
      className={cn(
        "grid grid-cols-1 items-center gap-4 border-static-800 border-b p-4 last:border-b-0 md:grid-cols-[minmax(0,2fr)_auto_auto_auto_auto]",
        // A disabled row is tinted, NOT dimmed. `opacity-60` over the whole row pushed
        // its text under the WCAG AA contrast floor — the a11y gate caught it — and a
        // disabled account is exactly the row an admin needs to read clearly. The
        // "Disabled" badge carries the state; the tint carries the at-a-glance grouping.
        user.disabled && "bg-static-900/40",
        className,
      )}
    >
      <div className="min-w-0">
        <div className="flex flex-wrap items-center gap-2">
          <p className="truncate font-medium">{user.name}</p>
          {isSelf && <Badge variant="tune">You</Badge>}
          {user.disabled && <Badge variant="onair">Disabled</Badge>}
        </div>
        {/* Local vs imported decides whether a password reset is even meaningful for
            this row, so it is stated rather than left for the admin to infer. */}
        <p className="mt-1 flex items-center gap-1.5 text-static-400 text-xs">
          {user.local ? (
            <KeyRound className="size-3" aria-hidden />
          ) : (
            <Server className="size-3" aria-hidden />
          )}
          {user.local ? "Local account" : "Media-server account"}
        </p>
      </div>

      <div className="flex items-center gap-2">
        <Label htmlFor={roleId} className="text-muted-foreground text-xs">
          Role
        </Label>
        <Select
          id={roleId}
          value={user.role}
          // Self-demotion is refused: an admin who removes their own admin role cannot
          // undo it from the UI, and if they are the last admin nobody can.
          disabled={busy || isSelf}
          title={isSelf ? "You cannot change your own role" : undefined}
          onChange={(e) => onRoleChange?.(e.target.value as "admin" | "member")}
        >
          <option value="member">member</option>
          <option value="admin">admin</option>
        </Select>
      </div>

      <div className="flex items-center gap-2">
        <Label htmlFor={quotaId} className="text-muted-foreground text-xs">
          Quota
        </Label>
        <Input
          id={quotaId}
          type="number"
          min={0}
          className="w-20"
          defaultValue={user.quota}
          disabled={busy}
          // Commit on blur, not per keystroke: typing "12" would otherwise PATCH a
          // quota of 1 on the way to 12.
          onBlur={(e) => {
            const next = Number(e.target.value);
            if (Number.isFinite(next) && next !== user.quota) onQuotaChange?.(next);
          }}
        />
      </div>

      <div className="flex items-center gap-2">
        <Checkbox
          id={autoId}
          checked={user.autoApprove}
          disabled={busy}
          onChange={(e) => onToggleAutoApprove?.(e.target.checked)}
        />
        <Label htmlFor={autoId} className="text-muted-foreground text-xs">
          Auto-approve
        </Label>
      </div>

      <div className="flex shrink-0 items-center gap-2">
        {onViewSessions && (
          <Button variant="ghost" size="sm" onClick={onViewSessions} disabled={busy}>
            <MonitorSmartphone aria-hidden />
            Sessions
          </Button>
        )}
        <Button
          variant={user.disabled ? "outline" : "ghost"}
          size="sm"
          disabled={busy || isSelf}
          title={isSelf ? "You cannot disable your own account" : undefined}
          onClick={() => onToggleDisabled?.(!user.disabled)}
        >
          {user.disabled ? "Enable" : "Disable"}
        </Button>
      </div>
    </div>
  );
};

export { UserRow };
