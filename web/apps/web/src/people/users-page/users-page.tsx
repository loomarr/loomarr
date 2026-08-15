import * as usersApi from "@loomarr/api/endpoints/users";
import type { PatchUserInputBody } from "@loomarr/api/models/patchUserInputBody";
import type { UserBody } from "@loomarr/api/models/userBody";
import { toProblem } from "@loomarr/api/mutator";
import { unwrap } from "@loomarr/api/unwrap";
import { useQueryClient } from "@tanstack/react-query";
import { UserPlus, UsersRound } from "lucide-react";
import { useId, useState } from "react";
import { toast } from "sonner";
import { useAuth } from "@/auth/use-auth";
import { EmptyState } from "@/components/loomarr/feedback/empty-state";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { SessionList } from "@/components/loomarr/people/session-list";
import { UserRow } from "@/components/loomarr/people/user-row";
import { PageHeader } from "@/components/loomarr/shell/page-header";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { CreateLocalPanel } from "../create-local-panel";
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
  // The admin reset path (§11): no current password — that is the point — so it relies
  // entirely on the caller's admin role, and it is a separate flow from the self-service
  // change on /account precisely so "prove you know it" never becomes optional.
  const [resetting, setResetting] = useState<UserBody>();
  const [newPassword, setNewPassword] = useState("");
  const [resetError, setResetError] = useState("");
  const [addMode, setAddMode] = useState<"local" | "import">();
  const resetId = useId();

  const users = usersApi.useListUsers({
    query: { refetchInterval: 60_000, refetchOnWindowFocus: true },
  });
  const invalidate = () => queryClient.invalidateQueries({ queryKey: usersApi.getListUsersQueryKey() });

  const patch = usersApi.usePatchUser({
    mutation: {
      onSuccess: () => toast.success("Person updated"),
      onError: (error) => toast.error(toProblem(error).title ?? "Couldn't update that person"),
      onSettled: () => {
        setBusyUser(undefined);
        void invalidate();
      },
    },
  });

  const sessions = usersApi.useListUserSessions(openSessions?.id ?? "", {
    query: { enabled: Boolean(openSessions), refetchInterval: 30_000, refetchOnWindowFocus: true },
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
      onSuccess: () => toast.success("Session revoked"),
      onError: (error) => toast.error(toProblem(error).title ?? "Couldn't revoke that session"),
      onSettled: () => {
        setRevoking(undefined);
        invalidateSessions();
      },
    },
  });

  const resetPassword = usersApi.useResetUserPassword({
    mutation: {
      onSuccess: () => {
        setResetting(undefined);
        setNewPassword("");
        setResetError("");
        // Their sessions are revoked server-side, so the list they might be looking at
        // is stale the moment this succeeds.
        invalidateSessions();
        toast.success("Password reset. Every session for that account has ended.");
      },
      onError: (e) => setResetError(toProblem(e).detail ?? "Couldn't reset that password."),
    },
  });

  const submitReset = () => {
    if (newPassword.length < 8) return setResetError("Use at least 8 characters.");
    setResetError("");
    if (resetting) resetPassword.mutate({ id: resetting.id, data: { next: newPassword } });
  };

  const edit = (id: string, body: PatchUserInputBody) => {
    setBusyUser(id);
    patch.mutate({ id, data: body });
  };

  if (users.error) return <ErrorState error={users.error} onRetry={() => users.refetch()} />;
  const rows = users.data?.status === 200 ? (users.data.data.users ?? []) : undefined;

  return (
    <div className="flex h-full min-h-0 flex-col">
      <PageHeader
        title="People"
        description="Control who can sign in, approve requests, and use automatic acquisition."
        actions={
          <>
            <Button
              variant={addMode === "local" ? "default" : "outline"}
              onClick={() => setAddMode((mode) => (mode === "local" ? undefined : "local"))}
            >
              <UserPlus aria-hidden />
              Add local account
            </Button>
            <Button
              variant={addMode === "import" ? "default" : "outline"}
              onClick={() => setAddMode((mode) => (mode === "import" ? undefined : "import"))}
            >
              <UsersRound aria-hidden />
              Import accounts
            </Button>
          </>
        }
      />

      <div className="flex flex-1 flex-col gap-6 overflow-auto p-6">
        {addMode === "local" ? (
          <CreateLocalPanel
            initiallyOpen
            onCreated={() => {
              void invalidate();
              setAddMode(undefined);
            }}
          />
        ) : null}
        {addMode === "import" ? (
          <ImportPanel
            onImported={() => {
              void invalidate();
            }}
          />
        ) : null}

        <Card>
          <div className="flex items-center justify-between gap-3 border-static-800 border-b px-4 py-3">
            <h2 className="font-medium text-sm">Access roster</h2>
            {rows ? (
              <span className="text-muted-foreground text-xs">
                {rows.length} {rows.length === 1 ? "person" : "people"}
              </span>
            ) : null}
          </div>
          {rows === undefined ? (
            <p className="p-4 text-muted-foreground text-sm">Loading users…</p>
          ) : rows.length === 0 ? (
            <EmptyState
              title="No users yet"
              description="Use Add local account or Import accounts above to grant someone access."
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
                onResetPassword={() => {
                  setResetting(u);
                  setNewPassword("");
                  setResetError("");
                }}
              />
            ))
          )}
        </Card>

        {/* Admin reset (§11). No current password — that is what distinguishes it from
          the self-service change on /account — so it leans entirely on the admin role,
          and every session for the target dies on success. */}
        {resetting && (
          <Card className="flex flex-col gap-3 p-4">
            <div className="flex items-center justify-between gap-3">
              <h2 className="font-semibold text-lg">{`Reset password, ${resetting.name}`}</h2>
              <Button variant="ghost" size="sm" onClick={() => setResetting(undefined)}>
                Close
              </Button>
            </div>
            <p className="text-muted-foreground text-sm">
              Set a new password for this account. They'll be signed out everywhere and will need the new one
              to get back in, so tell them what it is.
            </p>
            <div className="flex flex-col gap-1.5">
              <Label htmlFor={resetId}>New password</Label>
              <Input
                id={resetId}
                type="password"
                value={newPassword}
                onChange={(e) => setNewPassword(e.target.value)}
              />
            </div>
            {resetError && <p className="text-onair-300 text-sm">{resetError}</p>}
            <div className="flex gap-2">
              <Button onClick={submitReset} disabled={resetPassword.isPending}>
                Set new password
              </Button>
            </div>
          </Card>
        )}

        {openSessions && (
          <Card className="flex flex-col gap-3 p-4">
            <div className="flex items-center justify-between gap-3">
              <h2 className="font-semibold text-lg">{`Sessions, ${openSessions.name}`}</h2>
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
              sessions={unwrap(sessions.data, (b) => b.sessions) ?? []}
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
      </div>
    </div>
  );
};

export { UsersPage };
