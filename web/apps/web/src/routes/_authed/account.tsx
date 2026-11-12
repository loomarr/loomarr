import * as authApi from "@loomarr/api/endpoints/auth";
import * as usersApi from "@loomarr/api/endpoints/users";
import { toProblem } from "@loomarr/api/mutator";
import { unwrap } from "@loomarr/api/unwrap";
import { formatRelative } from "@loomarr/core/format";
import { changePasswordSchema } from "@loomarr/core/schemas";
import { Action, Card, FieldHint, Stack, StatusLabel } from "@loomarr/design-system";
import { ConfirmStep } from "@loomarr/ui";
import { createFileRoute } from "@tanstack/react-router";
import { useState } from "react";
import { toast } from "sonner";
import { meQueryOptions } from "@/auth/me-query";
import { useAuth } from "@/auth/use-auth";
import { PageHeader } from "@/components/loomarr/shell/page-header";
import { useAppForm } from "@/lib/app-form";
import { useDocumentTitle } from "@/lib/use-document-title";

// Your account (§11). The self-service half of local account management: change your
// own password, see where you're signed in, and revoke a session.
//
// This screen exists because the backend shipped without it — POST /v1/auth/password
// had 19 tests and no way to reach it from the app, which is the same
// capability-without-UI defect the surface map (§12) exists to prevent. The endpoint
// being correct was never the point; a user changing their password is.
//
// A viewer sees exactly what an admin sees here. Nothing on this page is privileged:
// it only ever acts on the caller's own account, which is why it takes no user id.

const PASSWORD_FIELDS = [
  { name: "current", label: "Current password", autoComplete: "current-password" },
  { name: "next", label: "New password", autoComplete: "new-password" },
  { name: "confirm", label: "Confirm new password", autoComplete: "new-password" },
] as const;

const AccountScreen = () => {
  const { user } = useAuth();
  useDocumentTitle("Your account");

  const [serverError, setServerError] = useState("");

  // A media-server account does not own an independent Loomarr password. Its captured
  // verifier is an outage fallback, not a credential this page may change.
  const isLocal = user?.local ?? false;

  const sessions = usersApi.useListUserSessions(user?.id ?? "", {
    query: { enabled: Boolean(user?.id) },
  });
  const sessionRows = unwrap(sessions.data, (b) => b.sessions) ?? [];

  const changePassword = authApi.useChangePassword({
    mutation: {
      onSuccess: () => {
        form.reset();
        setServerError("");
        // Every session ends, including this one — the backend revokes all of them
        // rather than sparing the caller's, because keeping a session means trusting
        // the request that might itself be the compromise. Say that plainly instead
        // of the friendlier-but-weaker "other sessions revoked, this one kept".
        toast.success("Password changed. Sign in again with the new one.");
      },
      onError: (e) => setServerError(toProblem(e).detail ?? "Couldn't change your password."),
    },
  });

  // Checked here as well as server-side: a mismatch is the user's typo, and making
  // them wait for a round-trip to learn it is worse than saying so immediately.
  const form = useAppForm({
    defaultValues: { current: "", next: "", confirm: "" },
    validators: { onSubmit: changePasswordSchema },
    onSubmit: ({ value }) => {
      setServerError("");
      changePassword.mutate({ data: { current: value.current, next: value.next } });
    },
  });

  const revoke = usersApi.useRevokeSession({
    mutation: {
      onSuccess: () => {
        void sessions.refetch();
        toast.success("Session revoked. That device signs in again next visit.");
      },
      onError: (e) => toast.error(toProblem(e).title ?? "Couldn't revoke that session"),
    },
  });
  // Signing a session out can't be taken back, so the first press only asks (one row at a time).
  const [confirmingSession, setConfirmingSession] = useState<string>();

  return (
    <Stack className="h-full min-h-0">
      <PageHeader
        title="Your account"
        description="Your sign-in, your password, and the devices holding a session."
      />

      <div className="max-w-form mx-auto flex w-full flex-1 flex-col gap-4 overflow-auto p-6">
        <Card alignItems="center" flexDirection="row" gap={12} padding={20}>
          <Stack
            align="center"
            className="bg-muted size-10 rounded-full font-medium"
            direction="row"
            justify="center"
          >
            {(user?.name ?? "?").slice(0, 2).toUpperCase()}
          </Stack>
          <div className="min-w-0">
            <p className="font-medium">{user?.name}</p>
            <Stack align="center" className="mt-0.5" direction="row" gap="1.5" wrap>
              <StatusLabel tone="off">{isLocal ? "Local account" : "Media-server account"}</StatusLabel>
              <StatusLabel tone={user?.role === "admin" ? "notice" : "off"}>
                {user?.role === "admin" ? "Admin" : "Member"}
              </StatusLabel>
            </Stack>
          </div>
        </Card>

        <Card gap={12} padding={20}>
          <h2 className="font-medium">Password</h2>
          {isLocal ? (
            <form
              className="flex flex-col gap-3"
              noValidate
              onSubmit={(e) => {
                e.preventDefault();
                void form.handleSubmit();
              }}
            >
              <p className="text-muted-foreground text-sm">
                This is a local Loomarr account, so Loomarr stores the password and you can change it here.
              </p>
              <Stack gap="inline">
                {PASSWORD_FIELDS.map(({ name, label, autoComplete }) => (
                  <form.AppField key={name} name={name}>
                    {(field) => <field.TextField autoComplete={autoComplete} label={label} type="password" />}
                  </form.AppField>
                ))}
              </Stack>
              {serverError && (
                <FieldHint role="alert" tone="error">
                  {serverError}
                </FieldHint>
              )}
              <Stack align="center" direction="row" gap="control">
                <Action disabled={changePassword.isPending} submit>
                  Change password
                </Action>
                {/* The honest consequence, stated before the click rather than after. */}
                <span className="text-static-400 text-xs">
                  changing it signs you out everywhere, including here
                </span>
              </Stack>
            </form>
          ) : (
            <p className="text-muted-foreground text-sm">
              Your media server owns this password. Loomarr stores only a non-reversible verifier after a
              successful sign-in
              {user?.offlineLogin ? ", so this account can sign in during a media-server outage" : ""}. Change
              the password on your media server; your next successful sign-in refreshes the verifier.
            </p>
          )}
        </Card>

        {/* The web mock's "Where you're signed in": the client ("Firefox on macOS") over when it
            was last used, a This device badge on your own, Sign out on the others. The mock's
            per-session city is dropped (geo-IP; the map's call on #1659). A session the API hasn't
            labelled yet keeps today's "Signed in …" title. */}
        {/* shrink-0: in the scrolling column, an overflow-hidden card would shrink and clip its rows (#1785). */}
        <Card flexShrink={0} overflow="hidden">
          <h2 className="border-border border-b px-4 py-3.5 text-sm font-semibold">Where you're signed in</h2>
          {sessions.isLoading && <p className="text-muted-foreground px-4 py-3 text-sm">Loading sessions…</p>}
          {!sessions.isLoading && sessionRows.length === 0 && (
            <p className="text-muted-foreground px-4 py-3 text-sm">No other sessions.</p>
          )}
          <ul>
            {sessionRows.map((s) => (
              <li
                key={s.id}
                className="border-border flex items-center gap-3 border-b px-4 py-3 last:border-b-0"
              >
                {/* The confirm takes the row's place, as Delete channel's replaces its choices. */}
                {confirmingSession === s.id ? (
                  <ConfirmStep
                    confirmLabel="Sign out"
                    disabled={revoke.isPending}
                    message={`Sign out ${s.clientLabel ?? "this session"}? That device has to sign in again.`}
                    onCancel={() => setConfirmingSession(undefined)}
                    onConfirm={() => {
                      setConfirmingSession(undefined);
                      revoke.mutate({ hash: s.id });
                    }}
                  />
                ) : (
                  <>
                    <div className="min-w-0 flex-1">
                      <p className="text-sm font-medium">
                        {s.clientLabel ?? `Signed in ${formatRelative(s.createdAt)}`}
                      </p>
                      {s.lastSeenAt !== undefined && (
                        <p className="text-muted-foreground mt-0.5 font-mono text-xs">
                          {formatRelative(s.lastSeenAt)}
                        </p>
                      )}
                    </div>
                    {s.current && <StatusLabel tone="notice">This device</StatusLabel>}
                    {/* No Sign out on your own session: the shell's Sign out is for that. */}
                    {!s.current && (
                      <Action
                        disabled={revoke.isPending}
                        onPress={() => setConfirmingSession(s.id)}
                        tone="danger"
                        variant="outline"
                      >
                        Sign out
                      </Action>
                    )}
                  </>
                )}
              </li>
            ))}
          </ul>
        </Card>
      </div>
    </Stack>
  );
};

const Route = createFileRoute("/_authed/account")({
  // The viewer's own sessions, so the list is there with the page rather than "Loading sessions…".
  // The page shows its own state for a failed read, so it is warmed.
  loader: async ({ context: { queryClient } }) => {
    const me = await queryClient.ensureQueryData(meQueryOptions());
    if (me.status !== 200) return;
    await queryClient.prefetchQuery(usersApi.getListUserSessionsQueryOptions(me.data.id));
  },
  component: AccountScreen,
});

export { Route };
