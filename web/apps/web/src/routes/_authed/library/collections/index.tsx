import * as libraryApi from "@loomarr/api/endpoints/library";
import { createFileRoute } from "@tanstack/react-router";
import { AdminOnly } from "@/auth/admin-only";
import { viewerIsAdmin } from "@/auth/me-query";
import { useDocumentTitle } from "@/lib/use-document-title";
import { LibraryPage } from "@/library/library-page";

// Collections (#2151 decision 28): the Library's fourth tab, the household's own collections and
// the ready-made sets. Admin-only like the rest of the Library.

const CollectionsScreen = () => {
  useDocumentTitle("Collections");
  return (
    <AdminOnly>
      <LibraryPage filter="collections" search="" view="collections" />
    </AdminOnly>
  );
};

const Route = createFileRoute("/_authed/library/collections/")({
  component: CollectionsScreen,
  // The collections, the status line and the Needs you count the page shows with them. Admin-only
  // like the page.
  loader: async ({ context: { queryClient } }) => {
    if (!(await viewerIsAdmin(queryClient))) return;
    await Promise.all([
      queryClient.ensureQueryData(libraryApi.getListCollectionsQueryOptions()),
      queryClient.ensureQueryData(libraryApi.getLibraryStatusQueryOptions()),
      queryClient.ensureQueryData(libraryApi.getListLibraryNeedsQueryOptions()),
    ]);
  },
});

export { Route };
