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
  // two), each answering for itself when it fails, so they are warmed. Admin-only, like the page.
  loader: async ({ context: { queryClient } }) => {
    if (!(await viewerIsAdmin(queryClient))) return;
    await Promise.all([
      queryClient.prefetchQuery(libraryApi.getListLibraryRootsQueryOptions()),
      queryClient.prefetchQuery(libraryApi.getLibraryStatusQueryOptions()),
    ]);
  },
});

export { Route };
