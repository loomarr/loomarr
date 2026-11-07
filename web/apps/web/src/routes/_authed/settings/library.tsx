import { createFileRoute } from "@tanstack/react-router";
import { SettingsLibrary } from "@/library/settings-library";
import { SettingsPage } from "@/settings/settings-page";

const LibrarySettingsPage = () => (
  <SettingsPage
    title="Library"
    description="Where your media lives and how Loomarr keeps up with it."
    entries={[]}
    blocks={[]}
  >
    <SettingsLibrary />
  </SettingsPage>
);

const Route = createFileRoute("/_authed/settings/library")({
  component: LibrarySettingsPage,
});

export { Route };
