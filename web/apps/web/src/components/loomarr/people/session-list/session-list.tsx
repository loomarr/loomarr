import { formatRelative, formatUntil, pluralize } from "@loomarr/core/format";
import { Action, Badge } from "@loomarr/design-system";
import { cn } from "@/lib/utils";
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
          <Action onPress={onRevokeAll} variant="outline">
            Sign out everywhere
          </Action>
        )}
      </div>

      <ul className="flex flex-col gap-2">
        {sessions.map((s) => (
          <li
            key={s.id}
            className="border-static-800 flex items-center justify-between gap-3 rounded-md border px-3 py-2"
          >
            <div className="min-w-0">
              <p className="flex items-center gap-2 text-sm">
                {`Signed in ${formatRelative(s.createdAt, now)}`}
                {s.current && (
                  <Badge shape="square" tone="tune">
                    This device
                  </Badge>
                )}
              </p>
              <p className="text-static-400 text-xs">{`Expires ${formatUntil(s.expiresAt, now)}`}</p>
            </div>
            <Action
              disabled={revoking === s.id}
              icon={revoking === s.id ? "loading" : undefined}
              onPress={() => onRevoke?.(s.id)}
              variant="ghost"
            >
              {s.current ? "Sign out (this device)" : "Sign out"}
            </Action>
          </li>
        ))}
      </ul>
    </div>
  );
};

export { SessionList };
