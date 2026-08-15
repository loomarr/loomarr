import * as authApi from "@loomarr/api/endpoints/auth";
import * as usersApi from "@loomarr/api/endpoints/users";
import { toProblem } from "@loomarr/api/mutator";
import { unwrap } from "@loomarr/api/unwrap";
import { createFileRoute } from "@tanstack/react-router";
import { useState } from "react";
import { toast } from "sonner";
import { useAuth } from "@/auth/use-auth";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
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

const relativeTime = (ms: number): string => {
  const diff = Date.now() - ms;
  const mins = Math.round(diff / 60000);
  if (mins < 1) return "just now";
  if (mins < 60) return `${mins}m ago`;
  const hours = Math.round(mins / 60);
  if (hours < 24) return `${hours}h ago`;
  return `${Math.round(hours / 24)}d ago`;
};

const AccountScreen = () => {
  const { user } = useAuth();
  useDocumentTitle("Your account");

  const [current, setCurrent] = useState("");
  const [next, setNext] = useState("");
  const [confirm, setConfirm] = useState("");
  const [formError, setFormError] = useState("");

  // A media-server account has no Loomarr-side password: the media server owns that
  // credential and Loomarr never sees it. Say so rather than offering a form that
  // can only 409 — and rather than implying we could change their Emby password.
  const isLocal = user?.local ?? false;

  const sessions = usersApi.useListUserSessions(user?.id ?? "", {
    query: { enabled: Boolean(user?.id) },
  });
  const sessionRows = unwrap(sessions.data, (b) => b.sessions) ?? [];

  const changePassword = authApi.useChangePassword({
    mutation: {
      onSuccess: () => {
        setCurrent("");
        setNext("");
        setConfirm("");
        setFormError("");
        // Every session ends, including this one — the backend revokes all of them
        // rather than sparing the caller's, because keeping a session means trusting
        // the request that might itself be the compromise. Say that plainly instead
        // of the friendlier-but-weaker "other sessions revoked, this one kept".
        toast.success("Password changed. Sign in again with the new one.");
      },
      onError: (e) => setFormError(toProblem(e).detail ?? "Couldn't change your password."),
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

  const submit = () => {
    // Checked here as well as server-side: a mismatch is the user's typo, and making
    // them wait for a round-trip to learn it is worse than saying so immediately.
    if (!current) return setFormError("Enter your current password.");
    if (next.length < 8) return setFormError("Use at least 8 characters.");
    if (next !== confirm) return setFormError("New passwords don't match.");
    setFormError("");
    changePassword.mutate({ data: { current, next } });
  };

  return (
    <div className="mx-auto flex w-full max-w-2xl flex-col gap-4 p-6">
      <header>
        <h1 className="font-semibold text-xl">Your account</h1>
        <p className="text-muted-foreground text-sm">
          Your sign-in, your password, and the devices holding a session.
        </p>
      </header>

      <Card className="flex items-center gap-3 p-5">
        <div className="flex size-10 items-center justify-center rounded-full bg-muted font-medium">
          {(user?.name ?? "?").slice(0, 2).toUpperCase()}
        </div>
        <div className="min-w-0">
          <p className="font-medium">{user?.name}</p>
          <div className="mt-0.5 flex flex-wrap items-center gap-1.5">
            <Badge variant={isLocal ? "tune" : "neutral"}>
              {isLocal ? "Local account" : "Media-server account"}
            </Badge>
            <Badge variant={user?.role === "admin" ? "lock" : "neutral"}>{user?.role}</Badge>
          </div>
        </div>
      </Card>

      <Card className="flex flex-col gap-3 p-5">
        <h2 className="font-medium">Password</h2>
        {isLocal ? (
          <>
            <p className="text-muted-foreground text-sm">
              This is a local Loomarr account, so Loomarr stores the password and you can change it here.
            </p>
            <div className="flex flex-col gap-2">
              <div className="flex flex-col gap-1.5">
                <Label htmlFor="acct-current">Current password</Label>
                <Input
                  id="acct-current"
                  type="password"
                  value={current}
                  onChange={(e) => setCurrent(e.target.value)}
                />
              </div>
              <div className="flex flex-col gap-1.5">
                <Label htmlFor="acct-next">New password</Label>
                <Input
                  id="acct-next"
                  type="password"
                  value={next}
                  onChange={(e) => setNext(e.target.value)}
                />
              </div>
              <div className="flex flex-col gap-1.5">
                <Label htmlFor="acct-confirm">Confirm new password</Label>
                <Input
                  id="acct-confirm"
                  type="password"
                  value={confirm}
                  onChange={(e) => setConfirm(e.target.value)}
                />
              </div>
            </div>
            {formError && <p className="text-onair-300 text-sm">{formError}</p>}
            <div className="flex items-center gap-3">
              <Button onClick={submit} disabled={changePassword.isPending}>
                Change password
              </Button>
              {/* The honest consequence, stated before the click rather than after. */}
              <span className="text-static-400 text-xs">
                changing it signs you out everywhere, including here
              </span>
            </div>
          </>
        ) : (
          <p className="text-muted-foreground text-sm">
            Your password lives on your media server and Loomarr never sees it. Change it there and the new
            one works here immediately.
          </p>
        )}
      </Card>

      <Card className="flex flex-col gap-3 p-5">
        <h2 className="font-medium">Where you're signed in</h2>
        {sessions.isLoading && <p className="text-muted-foreground text-sm">Loading sessions…</p>}
        {!sessions.isLoading && sessionRows.length === 0 && (
          <p className="text-muted-foreground text-sm">No other sessions.</p>
        )}
        <ul className="flex flex-col gap-2">
          {sessionRows.map((s) => (
            <li
              key={s.id}
              className="flex items-center justify-between gap-3 rounded-md border border-border px-3 py-2"
            >
              <div className="min-w-0">
                <p className="text-sm">
                  Signed in {relativeTime(s.createdAt)}
                  {s.current && <span className="ml-2 text-lock text-xs">this device</span>}
                </p>
                <p className="font-mono text-static-400 text-xs">expires {relativeTime(s.expiresAt)}</p>
              </div>
              {/* No Revoke on your own session: that is what signing out is for, and a
                  button that logs you out while labelled "Revoke" is a trap. */}
              {!s.current && (
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => revoke.mutate({ hash: s.id })}
                  disabled={revoke.isPending}
                >
                  Revoke
                </Button>
              )}
            </li>
          ))}
        </ul>
      </Card>
    </div>
  );
};

const Route = createFileRoute("/_authed/account")({
  component: AccountScreen,
});

export { Route };
