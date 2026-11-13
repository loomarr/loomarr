import * as libraryApi from "@loomarr/api/endpoints/library";
import { createFileRoute } from "@tanstack/react-router";
import { AdminOnly } from "@/auth/admin-only";
import { viewerIsAdmin } from "@/auth/me-query";
import { useDocumentTitle } from "@/lib/use-document-title";
import { LibraryPage } from "@/library/library-page";

// Needs you (#2151 decision 27): the Library page with one card per title Loomarr can't tell what
// it is, in place of the strip. Admin-only like the rest of the Library.

const NeedsYouScreen = () => {
  useDocumentTitle("Needs you");
  return (
    <AdminOnly>
      <LibraryPage filter="all" search="" view="needs" />
    </AdminOnly>
  );
};

const Route = createFileRoute("/_authed/library/needs")({
  component: NeedsYouScreen,
  // The Needs you cards and the status line above them. Admin-only like the page.
  loader: async ({ context: { queryClient } }) => {
    if (!(await viewerIsAdmin(queryClient))) return;
    await Promise.all([
      queryClient.ensureQueryData(libraryApi.getLibraryStatusQueryOptions()),
      queryClient.ensureQueryData(libraryApi.getListLibraryNeedsQueryOptions()),
    ]);
  },
});

export { Route };
