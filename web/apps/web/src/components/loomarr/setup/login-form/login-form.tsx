import { loginSchema } from "@loomarr/core/schemas";
import { Action, Stack, Text } from "@loomarr/design-system";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { useAppForm } from "@/lib/app-form";
import type { LoginFormProps } from "./login-form.type";

// LoginForm — the sign-in surface (§11). Local or imported media-server credentials
// land on Loomarr's own identity; the BE returns a single `invalid credentials` problem
// for a wrong password AND an un-imported user (no enumeration), so the UI never
// distinguishes them. Validation is loginSchema from packages/core, handed to TanStack
// Form as a Standard Schema validator (no resolver adapter — §14) and shared verbatim
// with the future mobile app (§4.3). Validating on submit, not on change, keeps errors
// out of the operator's way while they type. The block-level failure renders through
// ErrorState (RFC 7807 → words, §3), field errors inline.
// ssoErrorCopy turns a reason CODE from the callback into copy.
//
// ⚠ The vocabulary lives HERE, not on the server. The callback redirects with `?sso=<code>`
// and never a message, because reflecting server text into a URL the browser renders is how a
// redirect becomes a phishing surface — an attacker who can shape that parameter would be
// putting their words on our login page. A fixed map cannot be used that way, and an
// unrecognised code falls back to something neutral rather than echoing the input.
const SSO_ERRORS: Record<string, string> = {
  // The one an operator will actually hit: the provider was happy, Loomarr was not.
  sso_no_account:
    "Your provider signed you in, but there's no account here for you yet. Ask an admin to add you.",
  sso_expired: "That sign-in link expired. Try again.",
  sso_unavailable: "Single sign-on isn't set up on this install.",
  sso_provider_error: "Your provider didn't complete the sign-in.",
  sso_failed: "Single sign-on didn't work. Ask an admin to check the settings.",
};

const ssoErrorCopy = (code: string): string =>
  SSO_ERRORS[code] ?? "Single sign-on didn't work. Try signing in with a password.";

const LoginForm = ({
  onSubmit,
  isPending = false,
  error,
  onDevLogin,
  onSso,
  ssoError,
  className,
}: LoginFormProps) => {
  const form = useAppForm({
    defaultValues: { username: "", password: "" },
    validators: { onSubmit: loginSchema },
    onSubmit: ({ value }) => onSubmit(value),
  });

  return (
    <Stack
      className={className}
      gap="5"
      noValidate
      onSubmit={(e) => {
        e.preventDefault();
        form.handleSubmit();
      }}
      render="form"
      width="100%"
    >
      {error != null && <ErrorState error={error} />}

      <form.AppField name="username">
        {(field) => <field.TextField autoComplete="username" id="login-username" label="Username" />}
      </form.AppField>

      <form.AppField name="password">
        {(field) => (
          <field.TextField
            autoComplete="current-password"
            id="login-password"
            label="Password"
            type="password"
          />
        )}
      </form.AppField>

      <a
        className="text-signal -mt-3 self-end text-sm underline-offset-4 hover:underline"
        href="/forgot-password"
      >
        Forgot password?
      </a>

      <Action disabled={isPending} submit>
        {isPending ? "Signing in…" : "Sign in"}
      </Action>

      {/* Single sign-on (§11, V8), offered only when the server reports a configured
          provider. ⚠ It sits BELOW the password form, not above it: Loomarr's own sign-in
          always works alongside SSO rather than instead of it, and an install whose provider
          is down must not look like one nobody can enter. */}
      {onSso && (
        <Stack className="border-border border-t pt-4" gap="inline">
          {ssoError ? (
            <div role="alert">
              <Text as="p" textRole="compact" tone="danger">
                {ssoErrorCopy(ssoError)}
              </Text>
            </div>
          ) : null}
          <Action disabled={isPending} onPress={onSso} variant="outline">
            Sign in with your provider
          </Action>
        </Stack>
      )}

      {/* Development only (§11). Rendered solely when the SERVER reports
          LOOMARR_DEV_LOGIN=1, so it cannot appear in a shipped install. It says what it
          does rather than something friendly like "Quick sign in" — an affordance that
          skips authentication should read as the unusual thing it is, both for the
          maintainer using it and for anyone who finds it somewhere it shouldn't be. */}
      {onDevLogin && (
        <Stack align="center" className="border-border border-t pt-4" gap="1">
          <Action disabled={isPending} onPress={onDevLogin} variant="outline">
            Skip sign-in (dev)
          </Action>
          <Text as="p" textRole="hint" tone="secondary">
            <code>LOOMARR_DEV_LOGIN</code> is on. It signs you in as an admin with no password.
          </Text>
        </Stack>
      )}
    </Stack>
  );
};

export { LoginForm };
