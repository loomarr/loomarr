import { toProblem } from "@loomarr/api/mutator";
import { KeyRound, Server } from "lucide-react";
import { useId, useState } from "react";
import { SessionList } from "@/components/loomarr/people/session-list";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import type { PersonDetailProps } from "./person-detail.type";

const PersonDetail = ({
  user,
  isSelf,
  busy,
  sessions,
  sessionsLoading,
  revoking,
  onRoleChange,
  onQuotaChange,
  onToggleDisabled,
  onToggleAutoApprove,
  onRevokeSession,
  onResetPassword,
}: PersonDetailProps) => {
  const roleId = useId();
  const quotaId = useId();
  const autoId = useId();
  const passwordId = useId();
  const [resetOpen, setResetOpen] = useState(false);
  const [password, setPassword] = useState("");
  const [resetError, setResetError] = useState("");
  const [resetting, setResetting] = useState(false);

  const submitReset = async () => {
    if (!onResetPassword) return;
    if (password.length < 8) {
      setResetError("Use at least 8 characters.");
      return;
    }
    setResetError("");
    setResetting(true);
    try {
      await onResetPassword(password);
      setPassword("");
      setResetOpen(false);
    } catch (error) {
      setResetError(toProblem(error).detail ?? "Couldn't reset that password.");
    } finally {
      setResetting(false);
    }
  };

  return (
    <div className="flex flex-col gap-6 p-6">
      <section aria-labelledby="person-identity" className="flex flex-col gap-2">
        <h3 id="person-identity" className="font-semibold">
          Identity
        </h3>
        <div className="flex flex-wrap items-center gap-2">
          {isSelf && <Badge variant="tune">You</Badge>}
          {user.disabled && <Badge variant="onair">Disabled</Badge>}
        </div>
        <p className="flex items-center gap-2 text-muted-foreground text-sm">
          {user.local ? (
            <KeyRound className="size-4" aria-hidden />
          ) : (
            <Server className="size-4" aria-hidden />
          )}
          {user.local
            ? "Loomarr-local credentials"
            : user.offlineLogin
              ? "Media-server credentials · offline login ready"
              : "Media-server credentials · sign in once to enable offline login"}
        </p>
      </section>

      <section
        aria-labelledby="person-access"
        className="flex flex-col gap-3 border-static-800 border-t pt-6"
      >
        <div>
          <h3 id="person-access" className="font-semibold">
            Access
          </h3>
          <p className="text-muted-foreground text-sm">Role changes take effect immediately.</p>
        </div>
        <div className="flex flex-col gap-1.5">
          <Label htmlFor={roleId}>Role</Label>
          <Select
            value={user.role}
            disabled={busy || isSelf}
            onValueChange={(value) => onRoleChange?.(value as "admin" | "member")}
          >
            <SelectTrigger id={roleId} title={isSelf ? "You cannot change your own role" : undefined}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="member">Member</SelectItem>
              <SelectItem value="admin">Admin</SelectItem>
            </SelectContent>
          </Select>
          {isSelf && <p className="text-static-400 text-xs">You cannot change your own role.</p>}
        </div>
      </section>

      <section
        aria-labelledby="person-requests"
        className="flex flex-col gap-4 border-static-800 border-t pt-6"
      >
        <div>
          <h3 id="person-requests" className="font-semibold">
            Requests
          </h3>
          <p className="text-muted-foreground text-sm">
            Control how much this person may acquire and approve.
          </p>
        </div>
        <div className="grid grid-cols-[minmax(0,1fr)_7rem] items-end gap-4">
          <div>
            <p className="text-sm">Pending acquisitions</p>
            <p className="font-mono text-muted-foreground text-sm tabular-nums">{`${user.pendingAcquisitions} of ${user.effectiveQuota}`}</p>
          </div>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor={quotaId}>Quota</Label>
            <Input
              key={`${user.id}-${user.quota}`}
              id={quotaId}
              type="number"
              min={0}
              defaultValue={user.quota}
              placeholder={String(user.effectiveQuota)}
              disabled={busy}
              onBlur={(event) => {
                const next = Number(event.target.value);
                if (Number.isFinite(next) && next !== user.quota) onQuotaChange?.(next);
              }}
            />
          </div>
        </div>
        <div className="flex items-center gap-2">
          <Checkbox
            id={autoId}
            checked={user.autoApprove}
            disabled={busy}
            onChange={(event) => onToggleAutoApprove?.(event.target.checked)}
          />
          <Label htmlFor={autoId}>Auto-approve requests</Label>
        </div>
      </section>

      <section
        aria-labelledby="person-sessions"
        className="flex flex-col gap-3 border-static-800 border-t pt-6"
      >
        <div>
          <h3 id="person-sessions" className="font-semibold">
            Sessions
          </h3>
          <p className="text-muted-foreground text-sm">Review and end active sign-ins.</p>
        </div>
        <SessionList
          userName={user.name}
          sessions={sessions}
          loading={sessionsLoading}
          revoking={revoking}
          onRevoke={onRevokeSession}
        />
      </section>

      <section
        aria-labelledby="person-password"
        className="flex flex-col gap-3 border-static-800 border-t pt-6"
      >
        <div>
          <h3 id="person-password" className="font-semibold">
            Password
          </h3>
          <p className="text-muted-foreground text-sm">
            {user.local
              ? "Set a replacement Loomarr password and end every session."
              : "This password is managed by the connected media server."}
          </p>
        </div>
        {user.local &&
          onResetPassword &&
          (resetOpen ? (
            <div className="flex flex-col gap-3">
              <div className="flex flex-col gap-1.5">
                <Label htmlFor={passwordId}>New password</Label>
                <Input
                  id={passwordId}
                  type="password"
                  value={password}
                  onChange={(event) => setPassword(event.target.value)}
                />
              </div>
              {resetError && (
                <p className="text-onair-300 text-sm" role="alert">
                  {resetError}
                </p>
              )}
              <div className="flex flex-wrap gap-2">
                <Button onClick={() => void submitReset()} disabled={resetting}>
                  Set new password
                </Button>
                <Button
                  variant="ghost"
                  onClick={() => {
                    setResetOpen(false);
                    setPassword("");
                    setResetError("");
                  }}
                >
                  Cancel
                </Button>
              </div>
            </div>
          ) : (
            <Button variant="outline" className="self-start" onClick={() => setResetOpen(true)}>
              <KeyRound aria-hidden />
              Reset password
            </Button>
          ))}
      </section>

      <section
        aria-labelledby="person-danger"
        className="flex flex-col gap-3 rounded-lg border border-onair-700 bg-onair-950/20 p-4"
      >
        <div>
          <h3 id="person-danger" className="font-semibold text-onair-300">
            Danger zone
          </h3>
          <p className="text-muted-foreground text-sm">
            {user.disabled
              ? "Enable this account so the person can sign in again."
              : "Disable this account and end every active session immediately."}
          </p>
        </div>
        <Button
          variant={user.disabled ? "outline" : "destructive"}
          className="self-start"
          disabled={busy || isSelf}
          title={isSelf ? "You cannot disable your own account" : undefined}
          onClick={() => onToggleDisabled?.(!user.disabled)}
        >
          {user.disabled ? "Enable account" : "Disable account"}
        </Button>
        {isSelf && <p className="text-static-400 text-xs">You cannot disable your own account.</p>}
      </section>
    </div>
  );
};

export { PersonDetail };
