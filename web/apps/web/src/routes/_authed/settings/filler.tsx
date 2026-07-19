import { createFileRoute } from "@tanstack/react-router";
import { SettingsPage, useSettingsEntries } from "@/settings";

const FillerSettings = () => (
  <SettingsPage
    title="Filler"
    description="Commercials and bumpers: where they come from and how densely they play."
    entries={useSettingsEntries()}
    blocks={[{ group: "filler", title: "Filler library", check: "filler" }]}
  />
);

const Route = createFileRoute("/_authed/settings/filler")({
  component: FillerSettings,
});

export { Route };
