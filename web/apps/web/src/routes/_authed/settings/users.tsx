import { createFileRoute } from "@tanstack/react-router";
import { SecretsSettings, SettingsPage, useSettingsEntries } from "@/settings";

const UsersSettings = () => (
  <SettingsPage
    title="Users & security"
    description="Sessions, cookies, and how often Loomarr re-syncs media-server accounts."
    entries={useSettingsEntries()}
    blocks={[{ group: "users_security", title: "Sessions and sync" }]}
  />
);

const Route = createFileRoute("/_authed/settings/users")({
  component: UsersSettings,
});

export { Route };
