import { formatRelative, formatUntil, pluralize } from "@loomarr/core/format";
import { Action, Stack, StatusLabel } from "@loomarr/design-system";
import { ConfirmStep } from "@loomarr/ui";
import { useState } from "react";
import type { SessionListProps } from "./session-list.type";

// Session ids are opaque hashes, so this can't collide with a row's confirm.
const ALL = "all-sessions";

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
  // Signing a session out can't be taken back, so the first press only asks (one row at a time).
  const [confirming, setConfirming] = useState<string>();
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
    <Stack className={className} gap="control">
      <Stack align="center" direction="row" gap="control" justify="between">
        <p className="text-muted-foreground text-sm">
          {`${pluralize(sessions.length, "active session")} for ${userName}`}
        </p>
        {onRevokeAll && confirming !== ALL && (
          <Action onPress={() => setConfirming(ALL)} tone="danger" variant="outline">
            Sign out everywhere
          </Action>
        )}
      </Stack>
      {confirming === ALL && (
        <ConfirmStep
          confirmLabel="Sign out everywhere"
          message={`Sign ${userName} out of every device? Each one has to sign in again.`}
          onCancel={() => setConfirming(undefined)}
          onConfirm={() => {
            setConfirming(undefined);
            onRevokeAll?.();
          }}
        />
      )}

      <Stack gap="inline" render="ul">
        {sessions.map((s) =>
          // The confirm takes the row's place, as Delete channel's replaces its choices.
          confirming === s.id ? (
            <li key={s.id}>
              <ConfirmStep
                confirmLabel="Sign out"
                message={
                  s.current
                    ? "Sign out of this device? You'll need to sign in again."
                    : `Sign out the session from ${formatRelative(s.createdAt, now)}? That device has to sign in again.`
                }
                onCancel={() => setConfirming(undefined)}
                onConfirm={() => {
                  setConfirming(undefined);
                  onRevoke?.(s.id);
                }}
              />
            </li>
          ) : (
            <Stack
              align="center"
              className="border-static-800 rounded-md border px-3 py-2"
              direction="row"
              gap="control"
              justify="between"
              key={s.id}
              render="li"
            >
              <div className="min-w-0">
                <Stack align="center" className="text-sm" direction="row" gap="inline" render="p">
                  {`Signed in ${formatRelative(s.createdAt, now)}`}
                  {s.current && <StatusLabel tone="notice">This device</StatusLabel>}
                </Stack>
                <p className="text-static-400 text-xs">{`Expires ${formatUntil(s.expiresAt, now)}`}</p>
              </div>
              <Action
                disabled={revoking === s.id}
                icon={revoking === s.id ? "loading" : undefined}
                onPress={() => setConfirming(s.id)}
                tone="danger"
                variant="outline"
              >
                {s.current ? "Sign out (this device)" : "Sign out"}
              </Action>
            </Stack>
          ),
        )}
      </Stack>
    </Stack>
  );
};

export { SessionList };
