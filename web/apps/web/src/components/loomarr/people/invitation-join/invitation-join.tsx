import { CalendarClock, Loader2, LogIn, ShieldCheck, UserRound } from "lucide-react";
import { useState } from "react";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import type { InvitationJoinProps } from "./invitation-join.type";

const InvitationJoin = ({
  preview,
  isLoading = false,
  isRedeeming = false,
  error,
  onRedeem,
}: InvitationJoinProps) => {
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [confirmation, setConfirmation] = useState("");
  const [validation, setValidation] = useState<string>();

  if (isLoading) {
    return (
      <div className="flex min-h-48 items-center justify-center gap-2 text-muted-foreground" role="status">
        <Loader2 className="size-4 animate-spin" aria-hidden />
        Checking invitation…
      </div>
    );
  }
  if (!preview) {
    return error ? (
      <ErrorState error={error} />
    ) : (
      <div className="space-y-2 text-center">
        <h1 className="font-display font-semibold text-2xl">Invitation unavailable</h1>
        <p className="text-muted-foreground text-sm">
          This invitation is missing, expired, revoked, or already used. Ask an administrator for a new one.
        </p>
      </div>
    );
  }

  const library = preview.credentialPath === "library_password";
  const identity = preview.username || preview.displayName || "Invited account";
  const expires = new Intl.DateTimeFormat(undefined, { dateStyle: "medium", timeStyle: "short" }).format(
    new Date(preview.expiresAt),
  );

  const submit = () => {
    if (password.length < 8) {
      setValidation("Use at least 8 characters.");
      return;
    }
    if (library && username.trim() === "") {
      setValidation("Enter the username for the invited Library account.");
      return;
    }
    if (!library && password !== confirmation) {
      setValidation("Passwords need to match.");
      return;
    }
    setValidation(undefined);
    onRedeem(library ? { username: username.trim(), password } : { password });
  };

  return (
    <form
      className="flex w-full flex-col gap-5"
      noValidate
      onSubmit={(event) => {
        event.preventDefault();
        submit();
      }}
    >
      <div className="space-y-2 text-center">
        <h1 className="font-display font-semibold text-2xl">Join Loomarr</h1>
        <p className="text-muted-foreground text-sm">Review the account below before activating it.</p>
      </div>

      <div className="space-y-3 rounded-lg border border-border bg-muted/25 p-4">
        <div className="flex items-start gap-3">
          <UserRound className="mt-0.5 size-5 shrink-0 text-signal" aria-hidden />
          <div className="min-w-0 flex-1">
            <p className="text-muted-foreground text-xs uppercase tracking-wide">Invited identity</p>
            <p className="break-words font-medium">{identity}</p>
          </div>
          <Badge
            variant={preview.role === "admin" ? "onair" : "neutral"}
            className={preview.role === "admin" ? "bg-onair-tint-12" : undefined}
          >
            {preview.role}
          </Badge>
        </div>
        {preview.role === "admin" ? (
          <p className="flex gap-2 text-onair-300 text-sm">
            <ShieldCheck className="mt-0.5 size-4 shrink-0" aria-hidden />
            This account will have administrator access to people, settings, and approvals.
          </p>
        ) : null}
        <p className="flex gap-2 text-muted-foreground text-sm">
          <CalendarClock className="mt-0.5 size-4 shrink-0" aria-hidden />
          This invitation expires {expires}.
        </p>
      </div>

      <p className="text-muted-foreground text-sm">
        {library
          ? "Sign in with the current username and password for this Emby or Jellyfin account. The provider must verify it before Loomarr activates access."
          : "Create a Loomarr password for this reserved local account. Activation happens only when you submit this form."}
      </p>

      {error ? <ErrorState error={error} /> : null}

      {library ? (
        <div className="flex flex-col gap-2">
          <Label htmlFor="join-username">Library username</Label>
          <Input
            id="join-username"
            autoComplete="username"
            value={username}
            onChange={(event) => setUsername(event.target.value)}
          />
        </div>
      ) : null}

      <div className="flex flex-col gap-2">
        <Label htmlFor="join-password">{library ? "Library password" : "Create password"}</Label>
        <Input
          id="join-password"
          type="password"
          autoComplete={library ? "current-password" : "new-password"}
          value={password}
          onChange={(event) => setPassword(event.target.value)}
        />
      </div>

      {!library ? (
        <div className="flex flex-col gap-2">
          <Label htmlFor="join-confirm-password">Confirm password</Label>
          <Input
            id="join-confirm-password"
            type="password"
            autoComplete="new-password"
            value={confirmation}
            onChange={(event) => setConfirmation(event.target.value)}
          />
        </div>
      ) : null}

      {validation ? (
        <p className="text-onair-300 text-sm" role="alert">
          {validation}
        </p>
      ) : null}

      <Button type="submit" disabled={isRedeeming}>
        {isRedeeming ? <Loader2 className="animate-spin" aria-hidden /> : <LogIn aria-hidden />}
        {isRedeeming ? "Activating…" : "Activate account"}
      </Button>
    </form>
  );
};

export { InvitationJoin };
