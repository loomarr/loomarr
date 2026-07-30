import { createFileRoute } from "@tanstack/react-router";
import { SecretsSettings, SettingsPage, useSettingsEntries } from "@/settings";

// ⚠ States what SSO does NOT do, because the v2 mock drew two controls that D-F cut:
// "Create people on first sign-in" and "Admin group". They are ABSENT rather than disabled —
// a disabled control invites hunting for how to enable it, and neither can ever be enabled
// without contradicting the allowlist that is §11's source of truth.
//
// Saying so matters because most apps DO auto-create: an operator arriving from another
// tool's docs would otherwise read the absence as an oversight rather than the decision it is.
const SsoNote = () => (
  <section className="rounded-lg border border-border p-4">
    <h2 className="font-medium text-sm">How sign-in works with a provider</h2>
    <p className="mt-1 text-muted-foreground text-sm leading-relaxed">
      Your provider proves who someone is. The People list decides whether they can get in — signing in with
      your provider does not create an account here, and it does not set anyone's role. Add people under
      People first, then they can sign in either way.
    </p>
  </section>
);

const SecuritySettings = () => (
  <SettingsPage
    title="Security"
    description="Sessions, cookies, and how often Loomarr re-syncs media-server accounts."
    entries={useSettingsEntries()}
    blocks={[
      { group: "users_security", title: "Sessions and sync" },
      { group: "sso", title: "Single sign-on" },
    ]}
    footer={
      <>
        <SsoNote />
        <SecretsSettings />
      </>
    }
  />
);

const Route = createFileRoute("/_authed/settings/security")({
  component: SecuritySettings,
});

export { Route };
