import { createFileRoute } from "@tanstack/react-router";
import { SettingsPage, useSettingsEntries } from "@/settings";

const AdvancedSettings = () => (
  <SettingsPage
    title="Advanced"
    description="Timeouts, retention, and workers. Safe defaults — change only if you need to."
    entries={useSettingsEntries()}
    blocks={[{ group: "advanced", title: "Timeouts and retention" }]}
  />
);

const Route = createFileRoute("/_authed/settings/advanced")({
  component: AdvancedSettings,
});

export { Route };
