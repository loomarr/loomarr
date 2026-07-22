import { createFileRoute } from "@tanstack/react-router";
import { SettingsPage, useSettingsEntries } from "@/settings";

// Settings → Filler is the FIRST of three filler surfaces, and the description names the
// whole flow so the three don't read as unrelated pages: here you point Loomarr at the
// drop-folder and set how densely breaks play; the Filler page is the catalog you browse
// and tag; each channel then chooses its own filler on its Filler section.
const FillerSettings = () => (
  <SettingsPage
    title="Filler"
    description="Where your commercials and bumpers come from (the drop-folder) and how densely breaks play. Browse and tag the clips themselves on the Filler page; each channel picks its own filler on its Filler section."
    entries={useSettingsEntries()}
    blocks={[{ group: "filler", title: "Filler library", check: "filler" }]}
  />
);

const Route = createFileRoute("/_authed/settings/filler")({
  component: FillerSettings,
});

export { Route };
