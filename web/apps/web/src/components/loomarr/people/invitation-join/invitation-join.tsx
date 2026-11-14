import { redeemInvitationSchema } from "@loomarr/core/schemas";
import { Action, Icon, icons, LoadingStrip, Stack, StatusLabel, Text } from "@loomarr/design-system";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { useAppForm } from "@/lib/app-form";
import type { InvitationJoinProps } from "./invitation-join.type";

const InvitationJoin = ({
  preview,
  isLoading = false,
  isRedeeming = false,
  error,
  onRedeem,
}: InvitationJoinProps) => {
  const library = preview?.credentialPath === "library_password";
  const schema = redeemInvitationSchema(library);
  const form = useAppForm({
    defaultValues: { username: "", password: "", confirm: "" },
    validators: { onSubmit: schema },
    onSubmit: ({ value }) => {
      const { username, password } = schema.parse(value);
      onRedeem(library ? { username, password } : { password });
    },
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
        Checking invitation…
      </Stack>
    );
  }
  if (!preview) {
    return error ? (
      <ErrorState error={error} />
    ) : (
      <div className="space-y-2 text-center">
        <Text as="h1" strong textRole="display">
          Invitation unavailable
        </Text>
        <Text as="p" textRole="compact" tone="secondary">
          This invitation is missing, expired, revoked, or already used. Ask an administrator for a new one.
        </Text>
      </div>
    );
  }

  const identity = preview.username || preview.displayName || "Invited account";
  const expires = new Intl.DateTimeFormat(undefined, { dateStyle: "medium", timeStyle: "short" }).format(
    new Date(preview.expiresAt),
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
        <Text as="h1" strong textRole="display">
          Join Loomarr
        </Text>
        <Text as="p" textRole="compact" tone="secondary">
          Review the account below before activating it.
        </Text>
      </div>

      <div className="border-border bg-muted/25 space-y-3 rounded-lg border p-4">
        <Stack align="start" direction="row" gap="control">
          <Stack direction="row" marginTop={2} render="span">
            <Icon decorative glyph={icons.person} size="default" tone="primary" />
          </Stack>
          <div className="min-w-0 flex-1">
            <Text as="p" textRole="hint" tone="secondary">
              Invited identity
            </Text>
            <Text as="p" className="break-words" textRole="cardLabel">
              {identity}
            </Text>
          </div>
          <StatusLabel tone={preview.role === "admin" ? "error" : "off"}>
            {preview.role === "admin" ? "Admin" : "Member"}
          </StatusLabel>
        </Stack>
        {preview.role === "admin" ? (
          <Stack className="text-onair-300" direction="row" gap="inline" render="p">
            <Stack direction="row" marginTop={2} render="span">
              <Icon decorative glyph={icons.verified} size="compact" tone="inherit" />
            </Stack>
            <Text as="span" textRole="compact" tone="danger">
              This account will have administrator access to people, settings, and approvals.
            </Text>
          </Stack>
        ) : null}
        <Stack className="text-muted-foreground" direction="row" gap="inline" render="p">
          <Stack direction="row" marginTop={2} render="span">
            <Icon decorative glyph={icons.schedule} size="compact" tone="inherit" />
          </Stack>
          <Text as="span" textRole="compact" tone="secondary">
            This invitation expires {expires}.
          </Text>
        </Stack>
      </div>

      <Text as="p" textRole="compact" tone="secondary">
        {library
          ? "Sign in with the current username and password for this Emby or Jellyfin account. The provider must verify it before Loomarr activates access."
          : "Create a Loomarr password for this reserved local account. Activation happens only when you submit this form."}
      </Text>

      {error ? <ErrorState error={error} /> : null}

      {library ? (
        <form.AppField name="username">
          {(field) => <field.TextField autoComplete="username" id="join-username" label="Library username" />}
        </form.AppField>
      ) : null}

      <form.AppField name="password">
        {(field) => (
          <field.TextField
            autoComplete={library ? "current-password" : "new-password"}
            id="join-password"
            label={library ? "Library password" : "Create password"}
            type="password"
          />
        )}
      </form.AppField>

      {!library ? (
        <form.AppField name="confirm">
          {(field) => (
            <field.TextField
              autoComplete="new-password"
              id="join-confirm-password"
              label="Confirm password"
              type="password"
            />
          )}
        </form.AppField>
      ) : null}

      <Action disabled={isRedeeming} submit>
        {isRedeeming ? "Activating…" : "Activate account"}
      </Action>
    </Stack>
  );
};

export { InvitationJoin };
