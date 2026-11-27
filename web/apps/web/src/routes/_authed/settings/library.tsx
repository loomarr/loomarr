import * as libraryApi from "@loomarr/api/endpoints/library";
import { createFileRoute } from "@tanstack/react-router";
import { viewerIsAdmin } from "@/auth/me-query";
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
  // The folders and the library status the page is made of (`useLibraryFolders` reads the same
  // two), so it opens on them and a failed read is the route's error view. Admin-only, like the
  // page.
  loader: async ({ context: { queryClient } }) => {
    if (!(await viewerIsAdmin(queryClient))) return;
    await Promise.all([
      queryClient.ensureQueryData(libraryApi.getListLibraryRootsQueryOptions()),
      queryClient.ensureQueryData(libraryApi.getLibraryStatusQueryOptions()),
    ]);
  },
});

export { Route };
