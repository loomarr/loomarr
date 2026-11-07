import { passwordRecoveryRequestSchema, passwordRecoveryResetSchema } from "@loomarr/core/schemas";
import { CheckCircle2, KeyRound, Loader2, Mail } from "lucide-react";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { Button } from "@/components/ui/button";
import { useAppForm } from "@/lib/app-form";
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
  const form = useAppForm({
    defaultValues: { username: "" },
    validators: { onSubmit: passwordRecoveryRequestSchema },
    onSubmit: ({ value }) => onSubmit(passwordRecoveryRequestSchema.parse(value).username),
  });

  if (sent) {
    return (
      <div className="space-y-5 text-center" role="status">
        <CheckCircle2 className="text-signal mx-auto size-10" aria-hidden />
        <div className="space-y-2">
          <h1 className="font-display text-2xl font-semibold">Check your email</h1>
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
        void form.handleSubmit();
      }}
    >
      <div className="space-y-2 text-center">
        <Mail className="text-signal mx-auto size-8" aria-hidden />
        <h1 className="font-display text-2xl font-semibold">Forgot your password?</h1>
        <p className="text-muted-foreground text-sm">
          Enter your Loomarr username. We’ll email a reset link when this local account has a verified
          address.
        </p>
      </div>

      {error ? <ErrorState error={error} /> : null}

      <form.AppField name="username">
        {(field) => <field.TextField autoComplete="username" id="recovery-username" label="Username" />}
      </form.AppField>

      <Button type="submit" disabled={isPending}>
        {isPending ? <Loader2 className="animate-spin" aria-hidden /> : <Mail aria-hidden />}
        {isPending ? "Requesting…" : "Email reset link"}
      </Button>

      <div className="border-border space-y-2 border-t pt-4 text-center">
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
  const form = useAppForm({
    defaultValues: { password: "", confirm: "" },
    validators: { onSubmit: passwordRecoveryResetSchema },
    onSubmit: ({ value }) => onRedeem(value.password),
  });

  if (isLoading) {
    return (
      <div className="text-muted-foreground flex min-h-48 items-center justify-center gap-2" role="status">
        <Loader2 className="size-4 animate-spin" aria-hidden />
        Checking reset link…
      </div>
    );
  }
  if (succeeded) {
    return (
      <div className="space-y-5 text-center" role="status">
        <CheckCircle2 className="text-signal mx-auto size-10" aria-hidden />
        <div className="space-y-2">
          <h1 className="font-display text-2xl font-semibold">Password reset</h1>
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
        <h1 className="font-display text-2xl font-semibold">Reset link unavailable</h1>
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
        void form.handleSubmit();
      }}
    >
      <div className="space-y-2 text-center">
        <KeyRound className="text-signal mx-auto size-8" aria-hidden />
        <h1 className="font-display text-2xl font-semibold">Create a new password</h1>
        <p className="text-muted-foreground text-sm">This reset link expires {expiry}.</p>
      </div>

      {error ? <ErrorState error={error} /> : null}

      <form.AppField name="password">
        {(field) => (
          <field.TextField
            autoComplete="new-password"
            id="recovery-password"
            label="New password"
            type="password"
          />
        )}
      </form.AppField>
      <form.AppField name="confirm">
        {(field) => (
          <field.TextField
            autoComplete="new-password"
            id="recovery-confirm"
            label="Confirm new password"
            type="password"
          />
        )}
      </form.AppField>

      <Button type="submit" disabled={isRedeeming}>
        {isRedeeming ? <Loader2 className="animate-spin" aria-hidden /> : <KeyRound aria-hidden />}
        {isRedeeming ? "Resetting…" : "Reset password"}
      </Button>
    </form>
  );
};

export { PasswordRecoveryRequest, PasswordRecoveryReset };
