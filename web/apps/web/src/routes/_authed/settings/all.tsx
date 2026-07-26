import { createFileRoute } from "@tanstack/react-router";
import { SettingsPage, useSettingsEntries } from "@/settings";

// Settings → All settings (config-design §5, V9) — the escape hatch, and the old "Advanced"
// page's replacement. An operator who knows a key's name should never have to guess which page
// owns it, which is what a page named "Advanced" quietly became: a dumping ground for whatever
// did not fit elsewhere. V10 makes it searchable by key, group AND value.
const AllSettings = () => (
  <SettingsPage
    title="All settings"
    description="Every setting, including the advanced ones. Safe defaults — change only if you need to."
    entries={useSettingsEntries()}
    blocks={[{ group: "advanced", title: "Timeouts and retention" }]}
  />
);

const Route = createFileRoute("/_authed/settings/all")({
  component: AllSettings,
});

export { Route };
