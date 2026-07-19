import { createFileRoute } from "@tanstack/react-router";
import { SettingsPage, useSettingsEntries } from "@/settings";

const AdvancedSettings = () => (
  <SettingsPage
    title="Advanced"
    description="Timeouts, retention and workers. Sensible defaults — change them when you have a reason."
    entries={useSettingsEntries()}
    blocks={[{ group: "advanced", title: "Timeouts and retention" }]}
  />
);

const Route = createFileRoute("/_authed/settings/advanced")({
  component: AdvancedSettings,
});

export { Route };
