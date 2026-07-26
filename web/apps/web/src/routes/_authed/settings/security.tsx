import { createFileRoute } from "@tanstack/react-router";
import { SecretsSettings, SettingsPage, useSettingsEntries } from "@/settings";

const SecuritySettings = () => (
  <SettingsPage
    title="Security"
    description="Sessions, cookies, and how often Loomarr re-syncs media-server accounts."
    entries={useSettingsEntries()}
    blocks={[{ group: "users_security", title: "Sessions and sync" }]}
    footer={<SecretsSettings />}
  />
);

const Route = createFileRoute("/_authed/settings/security")({
  component: SecuritySettings,
});

export { Route };
