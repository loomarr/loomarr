import { type UserBody, usersApi } from "@loomarr/api";
import { useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { useAuth } from "@/auth";
import { EmptyState, ErrorState, SessionList, UserRow } from "@/components/loomarr";
import { Button, Card } from "@/components/ui";
import { ImportPanel } from "../import-panel";

// UsersPage — the §11 allowlist. You can sign in iff you have a row here, so this screen
// is the access-control surface: import who may sign in, set what they may spend, and end
// their sessions.
//
// Admin-only, enforced server-side (every route 403s for members). The route guard here
// is a courtesy so a member sees an explanation rather than a wall of failed requests.
const UsersPage = () => {
  const queryClient = useQueryClient();
  const { user: me } = useAuth();
  const [busyUser, setBusyUser] = useState<string>();
  const [openSessions, setOpenSessions] = useState<UserBody>();
  const [revoking, setRevoking] = useState<string>();

  const users = usersApi.useListUsers();
  const invalidate = () => queryClient.invalidateQueries({ queryKey: usersApi.getListUsersQueryKey() });

  const patch = usersApi.usePatchUser({
    mutation: {
      onSettled: () => {
        setBusyUser(undefined);
        void invalidate();
      },
    },
  });

  const sessions = usersApi.useListUserSessions(openSessions?.id ?? "", {
    query: { enabled: Boolean(openSessions) },
  });
  const invalidateSessions = () => {
    if (openSessions) {
      void queryClient.invalidateQueries({
        queryKey: usersApi.getListUserSessionsQueryKey(openSessions.id),
      });
    }
  };
  const revoke = usersApi.useRevokeSession({
    mutation: {
      onSettled: () => {
        setRevoking(undefined);
        invalidateSessions();
      },
    },
  });

  const edit = (id: string, body: Record<string, unknown>) => {
    setBusyUser(id);
    patch.mutate({ id, data: body });
  };

  if (users.error) return <ErrorState error={users.error} onRetry={() => users.refetch()} />;
  const rows = users.data?.status === 200 ? (users.data.data.users ?? []) : undefined;

  return (
    <div className="flex flex-col gap-6">
      <div>
        <h1 className="font-semibold text-xl">Users</h1>
        <p className="mt-1 text-muted-foreground text-sm">
          Who may sign in, what they may spend, and what they may approve. A media-server account grants no
          access until it is imported here.
        </p>
      </div>

      {patch.error != null && <ErrorState error={patch.error} />}
      {revoke.error != null && <ErrorState error={revoke.error} />}

      <Card>
        {rows === undefined ? (
          <p className="p-4 text-muted-foreground text-sm">Loading users…</p>
        ) : rows.length === 0 ? (
          <EmptyState
            title="No users yet"
            description="Import accounts from your media server below, and they can sign in with their existing password."
          />
        ) : (
          rows.map((u) => (
            <UserRow
              key={u.id}
              user={u}
              busy={busyUser === u.id}
              isSelf={u.id === me?.id}
              onRoleChange={(role) => edit(u.id, { role })}
              onQuotaChange={(quota) => edit(u.id, { quota })}
              onToggleAutoApprove={(autoApprove) => edit(u.id, { autoApprove })}
              onToggleDisabled={(disabled) => edit(u.id, { disabled })}
              onViewSessions={() => setOpenSessions(u)}
            />
          ))
        )}
      </Card>

      {openSessions && (
        <Card className="flex flex-col gap-3 p-4">
          <div className="flex items-center justify-between gap-3">
            <h2 className="font-semibold text-lg">{`Sessions — ${openSessions.name}`}</h2>
            <Button variant="ghost" size="sm" onClick={() => setOpenSessions(undefined)}>
              Close
            </Button>
          </div>
          <SessionList
            userName={openSessions.name}
            loading={sessions.isLoading}
            // `?? []` narrows the contract's nullable list (huma infers nullability from
            // Go's slice type); the handler always initializes it, so null never
            // actually arrives.
            sessions={sessions.data?.status === 200 ? (sessions.data.data.sessions ?? []) : []}
            revoking={revoking}
            onRevoke={(id) => {
              setRevoking(id);
              revoke.mutate({ hash: id });
            }}
          />
          {/* Disabling is the documented "kill every session now" path (§11), so it is
              named here rather than duplicated as a separate revoke-all call that the
              backend would treat differently. */}
          <p className="text-static-400 text-xs">Disabling this account ends every session immediately.</p>
        </Card>
      )}

      <ImportPanel onImported={invalidate} />
    </div>
  );
};

export { UsersPage };
