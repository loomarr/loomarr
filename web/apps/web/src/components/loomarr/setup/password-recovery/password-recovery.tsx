import { passwordRecoveryRequestSchema, passwordRecoveryResetSchema } from "@loomarr/core/schemas";
import { Action, Icon, icons, LoadingStrip, Stack, Text } from "@loomarr/design-system";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
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
        <Stack className="w-fit" direction="row" marginHorizontal="auto" render="span">
          <Icon decorative glyph={icons.passed} size="tv" tone="primary" />
        </Stack>
        <div className="space-y-2">
          <Text as="h1" strong textRole="display">
            Check your email
          </Text>
          <Text as="p" textRole="compact" tone="secondary">
            If that account can be recovered here, Loomarr sent a reset link to its verified address.
          </Text>
        </div>
        <SignInLink />
      </div>
    );
  }

  return (
    <Stack
      gap="5"
      noValidate
      onSubmit={(event) => {
        event.preventDefault();
        void form.handleSubmit();
      }}
      render="form"
      width="100%"
    >
      <div className="space-y-2 text-center">
        <Stack className="w-fit" direction="row" marginHorizontal="auto" render="span">
          <Icon decorative glyph={icons.mail} size="touch" tone="primary" />
        </Stack>
        <Text as="h1" strong textRole="display">
          Forgot your password?
        </Text>
        <Text as="p" textRole="compact" tone="secondary">
          Enter your Loomarr username. We’ll email a reset link when this local account has a verified
          address.
        </Text>
      </div>

      {error ? <ErrorState error={error} /> : null}

      <form.AppField name="username">
        {(field) => <field.TextField autoComplete="username" id="recovery-username" label="Username" />}
      </form.AppField>

      <Action disabled={isPending} submit>
        {isPending ? "Requesting…" : "Email reset link"}
      </Action>

      <div className="border-border space-y-2 border-t pt-4 text-center">
        <Text as="p" textRole="hint" tone="secondary">
          Emby or Jellyfin owns imported-account passwords. Change yours there, then sign in here once to
          refresh Loomarr’s offline verifier.
        </Text>
        <SignInLink />
      </div>
    </Stack>
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
      <Stack
        align="center"
        className="text-muted-foreground min-h-48"
        direction="row"
        gap="inline"
        justify="center"
        role="status"
      >
        <LoadingStrip decorative width={32} />
        Checking reset link…
      </Stack>
    );
  }
  if (succeeded) {
    return (
      <div className="space-y-5 text-center" role="status">
        <Stack className="w-fit" direction="row" marginHorizontal="auto" render="span">
          <Icon decorative glyph={icons.passed} size="tv" tone="primary" />
        </Stack>
        <div className="space-y-2">
          <Text as="h1" strong textRole="display">
            Password reset
          </Text>
          <Text as="p" textRole="compact" tone="secondary">
            Your new Loomarr password is ready. Every existing session has been signed out.
          </Text>
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
        <Text as="h1" strong textRole="display">
          Reset link unavailable
        </Text>
        <Text as="p" textRole="compact" tone="secondary">
          This link is missing, expired, revoked, or already used. Request a new one from the sign-in page.
        </Text>
        <SignInLink />
      </div>
    );
  }

  const expiry = new Intl.DateTimeFormat(undefined, { dateStyle: "medium", timeStyle: "short" }).format(
    new Date(expiresAt),
  );
  return (
    <Stack
      gap="5"
      noValidate
      onSubmit={(event) => {
        event.preventDefault();
        void form.handleSubmit();
      }}
      render="form"
      width="100%"
    >
      <div className="space-y-2 text-center">
        <Stack className="w-fit" direction="row" marginHorizontal="auto" render="span">
          <Icon decorative glyph={icons.key} size="touch" tone="primary" />
        </Stack>
        <Text as="h1" strong textRole="display">
          Create a new password
        </Text>
        <Text as="p" textRole="compact" tone="secondary">
          This reset link expires {expiry}.
        </Text>
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

      <Action disabled={isRedeeming} submit>
        {isRedeeming ? "Resetting…" : "Reset password"}
      </Action>
    </Stack>
  );
};

export { PasswordRecoveryRequest, PasswordRecoveryReset };
