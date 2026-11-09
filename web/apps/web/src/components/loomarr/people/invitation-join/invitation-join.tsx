import { redeemInvitationSchema } from "@loomarr/core/schemas";
import { Action, Icon, icons, LoadingStrip, StatusLabel } from "@loomarr/design-system";
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
      <div className="text-muted-foreground flex min-h-48 items-center justify-center gap-2" role="status">
        <LoadingStrip decorative width={32} />
        Checking invitation…
      </div>
    );
  }
  if (!preview) {
    return error ? (
      <ErrorState error={error} />
    ) : (
      <div className="space-y-2 text-center">
        <h1 className="font-display text-2xl font-semibold">Invitation unavailable</h1>
        <p className="text-muted-foreground text-sm">
          This invitation is missing, expired, revoked, or already used. Ask an administrator for a new one.
        </p>
      </div>
    );
  }

  const identity = preview.username || preview.displayName || "Invited account";
  const expires = new Intl.DateTimeFormat(undefined, { dateStyle: "medium", timeStyle: "short" }).format(
    new Date(preview.expiresAt),
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
        <h1 className="font-display text-2xl font-semibold">Join Loomarr</h1>
        <p className="text-muted-foreground text-sm">Review the account below before activating it.</p>
      </div>

      <div className="border-border bg-muted/25 space-y-3 rounded-lg border p-4">
        <div className="flex items-start gap-3">
          <span className="mt-0.5 flex">
            <Icon decorative glyph={icons.person} size="default" tone="primary" />
          </span>
          <div className="min-w-0 flex-1">
            <p className="text-muted-foreground text-xs">Invited identity</p>
            <p className="font-medium break-words">{identity}</p>
          </div>
          <StatusLabel tone={preview.role === "admin" ? "error" : "off"}>
            {preview.role === "admin" ? "Admin" : "Member"}
          </StatusLabel>
        </div>
        {preview.role === "admin" ? (
          <p className="text-onair-300 flex gap-2 text-sm">
            <span className="mt-0.5 flex">
              <Icon decorative glyph={icons.verified} size="compact" tone="inherit" />
            </span>
            This account will have administrator access to people, settings, and approvals.
          </p>
        ) : null}
        <p className="text-muted-foreground flex gap-2 text-sm">
          <span className="mt-0.5 flex">
            <Icon decorative glyph={icons.schedule} size="compact" tone="inherit" />
          </span>
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
    </form>
  );
};

export { InvitationJoin };
