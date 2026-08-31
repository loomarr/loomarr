import { CheckCircle2, KeyRound, Loader2, Mail } from "lucide-react";
import { useState } from "react";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import type { PasswordRecoveryRequestProps, PasswordRecoveryResetProps } from "./password-recovery.type";

const SignInLink = () => (
  <a className="text-signal text-sm underline-offset-4 hover:underline" href="/login">
    Back to sign in
  </a>
);

const PasswordRecoveryRequest = ({
  isPending = false,
  sent = false,
  error,
  onSubmit,
}: PasswordRecoveryRequestProps) => {
  const [username, setUsername] = useState("");
  const [validation, setValidation] = useState<string>();

  if (sent) {
    return (
      <div className="space-y-5 text-center" role="status">
        <CheckCircle2 className="mx-auto size-10 text-signal" aria-hidden />
        <div className="space-y-2">
          <h1 className="font-display font-semibold text-2xl">Check your email</h1>
          <p className="text-muted-foreground text-sm">
            If that account can be recovered here, Loomarr sent a reset link to its verified address.
          </p>
        </div>
        <SignInLink />
      </div>
    );
  }

  return (
    <form
      className="flex w-full flex-col gap-5"
      noValidate
      onSubmit={(event) => {
        event.preventDefault();
        const value = username.trim();
        if (value === "") {
          setValidation("Enter your username.");
          return;
        }
        setValidation(undefined);
        onSubmit(value);
      }}
    >
      <div className="space-y-2 text-center">
        <Mail className="mx-auto size-8 text-signal" aria-hidden />
        <h1 className="font-display font-semibold text-2xl">Forgot your password?</h1>
        <p className="text-muted-foreground text-sm">
          Enter your Loomarr username. We’ll email a reset link when this local account has a verified
          address.
        </p>
      </div>

      {error ? <ErrorState error={error} /> : null}

      <div className="flex flex-col gap-2">
        <Label htmlFor="recovery-username">Username</Label>
        <Input
          id="recovery-username"
          autoComplete="username"
          value={username}
          aria-invalid={validation ? "true" : undefined}
          onChange={(event) => setUsername(event.target.value)}
        />
        {validation ? (
          <p className="text-onair-300 text-sm" role="alert">
            {validation}
          </p>
        ) : null}
      </div>

      <Button type="submit" disabled={isPending}>
        {isPending ? <Loader2 className="animate-spin" aria-hidden /> : <Mail aria-hidden />}
        {isPending ? "Requesting…" : "Email reset link"}
      </Button>

      <div className="space-y-2 border-border border-t pt-4 text-center">
        <p className="text-muted-foreground text-xs">
          Emby or Jellyfin owns imported-account passwords. Change yours there, then sign in here once to
          refresh Loomarr’s offline verifier.
        </p>
        <SignInLink />
      </div>
    </form>
  );
};

const PasswordRecoveryReset = ({
  expiresAt,
  isLoading = false,
  isRedeeming = false,
  succeeded = false,
  error,
  onRedeem,
}: PasswordRecoveryResetProps) => {
  const [password, setPassword] = useState("");
  const [confirmation, setConfirmation] = useState("");
  const [validation, setValidation] = useState<string>();

  if (isLoading) {
    return (
      <div className="flex min-h-48 items-center justify-center gap-2 text-muted-foreground" role="status">
        <Loader2 className="size-4 animate-spin" aria-hidden />
        Checking reset link…
      </div>
    );
  }
  if (succeeded) {
    return (
      <div className="space-y-5 text-center" role="status">
        <CheckCircle2 className="mx-auto size-10 text-signal" aria-hidden />
        <div className="space-y-2">
          <h1 className="font-display font-semibold text-2xl">Password reset</h1>
          <p className="text-muted-foreground text-sm">
            Your new Loomarr password is ready. Every existing session has been signed out.
          </p>
        </div>
        <SignInLink />
      </div>
    );
  }
  if (expiresAt == null) {
    return error ? (
      <ErrorState error={error} />
    ) : (
      <div className="space-y-3 text-center">
        <h1 className="font-display font-semibold text-2xl">Reset link unavailable</h1>
        <p className="text-muted-foreground text-sm">
          This link is missing, expired, revoked, or already used. Request a new one from the sign-in page.
        </p>
        <SignInLink />
      </div>
    );
  }

  const expiry = new Intl.DateTimeFormat(undefined, { dateStyle: "medium", timeStyle: "short" }).format(
    new Date(expiresAt),
  );
  return (
    <form
      className="flex w-full flex-col gap-5"
      noValidate
      onSubmit={(event) => {
        event.preventDefault();
        if (password.length < 8) {
          setValidation("Use at least 8 characters.");
          return;
        }
        if (password !== confirmation) {
          setValidation("Passwords need to match.");
          return;
        }
        setValidation(undefined);
        onRedeem(password);
      }}
    >
      <div className="space-y-2 text-center">
        <KeyRound className="mx-auto size-8 text-signal" aria-hidden />
        <h1 className="font-display font-semibold text-2xl">Create a new password</h1>
        <p className="text-muted-foreground text-sm">This reset link expires {expiry}.</p>
      </div>

      {error ? <ErrorState error={error} /> : null}

      <div className="flex flex-col gap-2">
        <Label htmlFor="recovery-password">New password</Label>
        <Input
          id="recovery-password"
          type="password"
          autoComplete="new-password"
          value={password}
          onChange={(event) => setPassword(event.target.value)}
        />
      </div>
      <div className="flex flex-col gap-2">
        <Label htmlFor="recovery-confirm">Confirm new password</Label>
        <Input
          id="recovery-confirm"
          type="password"
          autoComplete="new-password"
          value={confirmation}
          onChange={(event) => setConfirmation(event.target.value)}
        />
      </div>

      {validation ? (
        <p className="text-onair-300 text-sm" role="alert">
          {validation}
        </p>
      ) : null}

      <Button type="submit" disabled={isRedeeming}>
        {isRedeeming ? <Loader2 className="animate-spin" aria-hidden /> : <KeyRound aria-hidden />}
        {isRedeeming ? "Resetting…" : "Reset password"}
      </Button>
    </form>
  );
};

export { PasswordRecoveryRequest, PasswordRecoveryReset };
