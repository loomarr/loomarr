import * as libraryApi from "@loomarr/api/endpoints/library";
import { createFileRoute } from "@tanstack/react-router";
import { AdminOnly } from "@/auth/admin-only";
import { viewerIsAdmin } from "@/auth/me-query";
import { useDocumentTitle } from "@/lib/use-document-title";
import { LibraryNeedsPage } from "@/library/library-needs-page";

// Needs you (#2151 decision 27): its own page, with one card per title Loomarr can't tell what it
// is. Admin-only, unlike the wall, a title's page and Collections.

const NeedsYouScreen = () => {
  useDocumentTitle("Needs you");
  return (
    <AdminOnly>
      <LibraryNeedsPage />
    </AdminOnly>
  );
};

const Route = createFileRoute("/_authed/library/needs")({
  component: NeedsYouScreen,
  // The Needs you cards and the library's status. Admin-only like the page.
  loader: async ({ context: { queryClient } }) => {
    if (!(await viewerIsAdmin(queryClient))) return;
    await Promise.all([
      queryClient.ensureQueryData(libraryApi.getLibraryStatusQueryOptions()),
      queryClient.ensureQueryData(libraryApi.getListLibraryNeedsQueryOptions()),
    ]);
  },
});

export { Route };
