import { ChevronDown, KeyRound, MonitorSmartphone, ShieldCheck } from "lucide-react";
import { useState } from "react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { cn } from "@/lib/utils";
import type { UserRowProps } from "./user-row.type";

type Confirmation = "demote" | "disable";

// UserRow defaults to a readable roster row. The independent PATCH controls remain immediate,
// but live behind Manage so a household list is not a wall of repeated form fields and danger
// buttons. This also gives the actions room to wrap on a phone instead of overflowing the page.
const UserRow = ({
  user,
  busy,
  isSelf,
  onRoleChange,
  onQuotaChange,
  onToggleDisabled,
  onToggleAutoApprove,
  onViewSessions,
  onResetPassword,
  className,
}: UserRowProps) => {
  const [expanded, setExpanded] = useState(false);
  const [confirmation, setConfirmation] = useState<Confirmation>();
  const [quotaError, setQuotaError] = useState(false);
  const roleId = `role-${user.id}`;
  const quotaId = `quota-${user.id}`;
  const autoId = `auto-${user.id}`;

  const confirm = () => {
    if (confirmation === "demote") onRoleChange?.("member");
    if (confirmation === "disable") onToggleDisabled?.(true);
    setConfirmation(undefined);
  };

  return (
    <div
      className={cn(
        "border-static-800 border-b p-4 last:border-b-0",
        user.disabled && "bg-static-900/40",
        className,
      )}
    >
      <div className="flex flex-wrap items-center gap-3">
        <div className="min-w-40 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <p className="truncate font-medium">{user.name}</p>
            {isSelf && <Badge variant="tune">You</Badge>}
            {user.disabled && <Badge variant="onair">Disabled</Badge>}
            {user.autoApprove && <Badge variant="signal">Auto-approve</Badge>}
          </div>
          <p className="mt-1 flex flex-wrap items-center gap-x-3 gap-y-1 text-static-400 text-xs">
            <span className="inline-flex items-center gap-1.5">
              {user.local ? (
                <KeyRound className="size-3" aria-hidden />
              ) : (
                <ShieldCheck className="size-3" aria-hidden />
              )}
              {user.local ? "Loomarr password" : "External sign-in"}
            </span>
            <span>{user.role === "admin" ? "Admin" : "Member"}</span>
            <span className="font-mono tabular-nums">
              {user.usageAvailable
                ? `${user.pendingAcquisitions} of ${user.effectiveQuota} pending`
                : `Pending usage unavailable · limit ${user.effectiveQuota}`}
            </span>
          </p>
        </div>

        <Button
          type="button"
          variant="outline"
          size="sm"
          aria-expanded={expanded}
          onClick={() => setExpanded((value) => !value)}
        >
          Manage
          <ChevronDown className={cn("transition-transform", expanded && "rotate-180")} aria-hidden />
        </Button>
      </div>

      {expanded ? (
        <div className="mt-4 grid gap-4 border-static-800 border-t pt-4 sm:grid-cols-2 xl:grid-cols-4">
          <div className="flex flex-col gap-1.5">
            <Label htmlFor={roleId} className="text-muted-foreground text-xs">
              Role
            </Label>
            <Select
              value={user.role}
              disabled={busy || isSelf}
              onValueChange={(value) => {
                const role = value as "admin" | "member";
                if (user.role === "admin" && role === "member") setConfirmation("demote");
                else onRoleChange?.(role);
              }}
            >
              <SelectTrigger id={roleId} title={isSelf ? "You cannot change your own role" : undefined}>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="member">Member</SelectItem>
                <SelectItem value="admin">Admin</SelectItem>
              </SelectContent>
            </Select>
          </div>

          <div className="flex flex-col gap-1.5">
            <Label htmlFor={quotaId} className="text-muted-foreground text-xs">
              Custom pending limit
            </Label>
            <Input
              key={`${user.id}-${user.quota}`}
              id={quotaId}
              type="number"
              min={0}
              defaultValue={user.quota || ""}
              placeholder={`Default (${user.effectiveQuota})`}
              disabled={busy}
              aria-invalid={quotaError || undefined}
              aria-describedby={quotaError ? `${quotaId}-error` : `${quotaId}-hint`}
              onChange={() => setQuotaError(false)}
              onBlur={(event) => {
                const raw = event.target.value.trim();
                const next = raw === "" ? 0 : Number(raw);
                if (!Number.isInteger(next) || next < 0) {
                  setQuotaError(true);
                  return;
                }
                setQuotaError(false);
                if (next !== user.quota) onQuotaChange?.(next);
              }}
            />
            {quotaError ? (
              <p id={`${quotaId}-error`} className="text-onair-300 text-xs" role="alert">
                Enter a whole number of zero or more.
              </p>
            ) : (
              <p id={`${quotaId}-hint`} className="text-static-400 text-xs">
                Leave blank to follow the system default.
              </p>
            )}
          </div>

          <div className="flex items-center gap-2 sm:self-start sm:pt-6">
            <Checkbox
              id={autoId}
              checked={user.autoApprove}
              disabled={busy}
              onChange={(event) => onToggleAutoApprove?.(event.target.checked)}
            />
            <Label htmlFor={autoId} className="font-normal text-sm">
              Approve automatically
            </Label>
          </div>

          <div className="flex flex-wrap items-center gap-2 xl:justify-end xl:self-start xl:pt-5">
            {onViewSessions ? (
              <Button variant="ghost" size="sm" onClick={onViewSessions} disabled={busy}>
                <MonitorSmartphone aria-hidden />
                Sessions
              </Button>
            ) : null}
            {onResetPassword && user.local ? (
              <Button variant="ghost" size="sm" onClick={onResetPassword} disabled={busy}>
                <KeyRound aria-hidden />
                Reset password
              </Button>
            ) : null}
            <Button
              variant={user.disabled ? "outline" : "ghost"}
              size="sm"
              disabled={busy || isSelf}
              title={isSelf ? "You cannot disable your own account" : undefined}
              onClick={() => {
                if (user.disabled) onToggleDisabled?.(false);
                else setConfirmation("disable");
              }}
            >
              {user.disabled ? "Enable" : "Disable"}
            </Button>
          </div>
        </div>
      ) : null}

      <Dialog open={confirmation != null} onOpenChange={(open) => !open && setConfirmation(undefined)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>
              {confirmation === "demote" ? `Make ${user.name} a member?` : `Disable ${user.name}?`}
            </DialogTitle>
            <DialogDescription>
              {confirmation === "demote"
                ? "They will lose access to approvals, settings, channels, and people management."
                : "They will be signed out everywhere immediately and cannot sign in until re-enabled."}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="ghost" onClick={() => setConfirmation(undefined)}>
              Cancel
            </Button>
            <Button variant="destructive" onClick={confirm}>
              {confirmation === "demote" ? "Make member" : "Disable account"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
};

export { UserRow };
