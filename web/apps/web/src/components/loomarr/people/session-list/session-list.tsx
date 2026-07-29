import { formatRelative, formatUntil, pluralize } from "@loomarr/core";
import { Loader2 } from "lucide-react";
import { Badge, Button } from "@/components/ui";
import { cn } from "@/lib";
import type { SessionListProps } from "./session-list.type";

// SessionList — who is signed in as this user, and the ability to end it (§11: sessions
// are revocable rows, and disabling a user kills them immediately). Surfaced because
// revocation was previously something the system did with nothing able to observe it.
//
// The caller's own session is labelled rather than hidden: an admin reviewing their
// account should see every live session, and the label is what stops them from signing
// themselves out by accident.
const SessionList = ({
  userName,
  sessions,
  revoking,
  loading,
  onRevoke,
  onRevokeAll,
  now = Date.now(),
  className,
}: SessionListProps) => {
  if (loading) {
    return <p className="text-muted-foreground text-sm">Loading sessions…</p>;
  }
  if (sessions.length === 0) {
    return (
      <p className="text-muted-foreground text-sm">
        {`${userName} has no active sessions. They are not signed in anywhere.`}
      </p>
    );
  }

  return (
    <div className={cn("flex flex-col gap-3", className)}>
      <div className="flex items-center justify-between gap-3">
        <p className="text-muted-foreground text-sm">
          {`${pluralize(sessions.length, "active session")} for ${userName}`}
        </p>
        {onRevokeAll && (
          <Button variant="outline" size="sm" onClick={onRevokeAll}>
            Sign out everywhere
          </Button>
        )}
      </div>

      <ul className="flex flex-col gap-2">
        {sessions.map((s) => (
          <li
            key={s.id}
            className="flex items-center justify-between gap-3 rounded-md border border-static-800 px-3 py-2"
          >
            <div className="min-w-0">
              <p className="flex items-center gap-2 text-sm">
                {`Signed in ${formatRelative(s.createdAt, now)}`}
                {s.current && <Badge variant="tune">This device</Badge>}
              </p>
              <p className="text-static-400 text-xs">{`Expires ${formatUntil(s.expiresAt, now)}`}</p>
            </div>
            <Button variant="ghost" size="sm" disabled={revoking === s.id} onClick={() => onRevoke?.(s.id)}>
              {revoking === s.id ? <Loader2 className="animate-spin" aria-hidden /> : null}
              {s.current ? "Sign out (this device)" : "Sign out"}
            </Button>
          </li>
        ))}
      </ul>
    </div>
  );
};

export { SessionList };
