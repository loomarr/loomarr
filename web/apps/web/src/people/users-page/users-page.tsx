import * as settingsApi from "@loomarr/api/endpoints/settings";
import * as usersApi from "@loomarr/api/endpoints/users";
import { unwrap } from "@loomarr/api/unwrap";
import { useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { useAuth } from "@/auth/use-auth";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { ImportDialog } from "@/components/loomarr/people/import-dialog";
import { PeopleRoster } from "@/components/loomarr/people/people-roster";
import { PersonDetail } from "@/components/loomarr/people/person-detail";
import { PageHeader } from "@/components/loomarr/shell/page-header";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { CreateLocalPanel } from "../create-local-panel";

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
  const [selectedId, setSelectedId] = useState<string>();
  const [revoking, setRevoking] = useState<string>();

  const users = usersApi.useListUsers();
  const invalidate = () => queryClient.invalidateQueries({ queryKey: usersApi.getListUsersQueryKey() });
  const settings = settingsApi.useSettingsList();
  const importAvailable =
    settings.data?.status === 200 ? Boolean(settings.data.data.features?.user_sync) : false;
  const candidates = usersApi.useImportCandidates({ query: { enabled: importAvailable } });
  const importUsers = usersApi.useImportUsers();
  const syncUsers = usersApi.useSyncUsers();

  const patch = usersApi.usePatchUser({
    mutation: {
      onSettled: () => {
        setBusyUser(undefined);
        void invalidate();
      },
    },
  });

  const sessions = usersApi.useListUserSessions(selectedId ?? "", {
    query: { enabled: Boolean(selectedId) },
  });
  const invalidateSessions = () => {
    if (selectedId) {
      void queryClient.invalidateQueries({
        queryKey: usersApi.getListUserSessionsQueryKey(selectedId),
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

  const resetPassword = usersApi.useResetUserPassword();

  const edit = (id: string, body: Record<string, unknown>) => {
    setBusyUser(id);
    patch.mutate({ id, data: body });
  };

  if (users.error) return <ErrorState error={users.error} onRetry={() => users.refetch()} />;
  const rows = users.data?.status === 200 ? (users.data.data.users ?? []) : undefined;
  const selected = rows?.find((user) => user.id === selectedId);

  return (
    <div className="flex h-full min-h-0 flex-col">
      <PageHeader
        title="People"
        description="Who may sign in, what they may spend, and what they may approve. An account grants no access until you add it here: by importing a media-server account, or creating a local one."
        actions={
          <ImportDialog
            available={importAvailable}
            candidates={
              importAvailable && !candidates.isLoading
                ? (unwrap(candidates.data, (body) => body.candidates) ?? [])
                : undefined
            }
            candidateError={candidates.error}
            mutationError={importUsers.error ?? syncUsers.error}
            importing={importUsers.isPending}
            syncing={syncUsers.isPending}
            onRetry={() => candidates.refetch()}
            onImport={async (ids) => {
              await importUsers.mutateAsync({ data: { ids } });
              await Promise.all([invalidate(), candidates.refetch()]);
            }}
            onSync={async () => {
              await syncUsers.mutateAsync();
              await Promise.all([invalidate(), candidates.refetch()]);
            }}
          />
        }
      />

      <div className="flex min-h-0 flex-1 flex-col gap-6 overflow-auto p-6">
        {patch.error != null && <ErrorState error={patch.error} />}
        {revoke.error != null && <ErrorState error={revoke.error} />}

        <PeopleRoster
          users={rows}
          selectedId={selectedId}
          selfId={me?.id}
          onSelect={(user) => setSelectedId(user.id)}
        />

        <Sheet
          open={Boolean(selected)}
          onOpenChange={(open) => !open && setSelectedId(undefined)}
          swipeDirection="right"
        >
          {selected && (
            <SheetContent>
              <SheetHeader>
                <SheetTitle>{selected.name}</SheetTitle>
                <SheetDescription>
                  Manage this person's access, request policy, credentials, and sessions.
                </SheetDescription>
              </SheetHeader>
              <PersonDetail
                user={selected}
                busy={busyUser === selected.id}
                isSelf={selected.id === me?.id}
                sessionsLoading={sessions.isLoading}
                sessions={unwrap(sessions.data, (body) => body.sessions) ?? []}
                revoking={revoking}
                onRoleChange={(role) => edit(selected.id, { role })}
                onQuotaChange={(quota) => edit(selected.id, { quota })}
                onToggleAutoApprove={(autoApprove) => edit(selected.id, { autoApprove })}
                onToggleDisabled={(disabled) => edit(selected.id, { disabled })}
                onRevokeSession={(id) => {
                  setRevoking(id);
                  revoke.mutate({ hash: id });
                }}
                onResetPassword={
                  selected.local
                    ? async (next) => {
                        await resetPassword.mutateAsync({ id: selected.id, data: { next } });
                        invalidateSessions();
                      }
                    : undefined
                }
              />
            </SheetContent>
          )}
        </Sheet>

        {/* Two ways into the allowlist, both explicit admin actions (§11): import an
          existing media-server account, or mint a local one for someone who has none. */}
        <CreateLocalPanel onCreated={invalidate} />
      </div>
    </div>
  );
};

export { UsersPage };
