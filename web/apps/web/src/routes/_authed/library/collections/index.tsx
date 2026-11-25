import * as libraryApi from "@loomarr/api/endpoints/library";
import { createFileRoute } from "@tanstack/react-router";
import { viewerIsAdmin } from "@/auth/me-query";
import { useDocumentTitle } from "@/lib/use-document-title";
import { libraryCountsQueryOptions } from "@/library/library-items-query";
import { LibraryPage } from "@/library/library-page";

// Collections (#2151 decision 28): the Library's fourth tab, the household's own collections and,
// for an admin, the ready-made sets. A member reads the collections (#4476) and changes nothing.

const CollectionsScreen = () => {
  useDocumentTitle("Collections");
  return <LibraryPage filter="collections" search="" view="collections" />;
};

const Route = createFileRoute("/_authed/library/collections/")({
  component: CollectionsScreen,
  // The collections, the tabs' counts, and for an admin the status line over them. A member's
  // status request would 403, so only an admin's is asked.
  loader: async ({ context: { queryClient } }) => {
    const collections = queryClient.ensureQueryData(libraryApi.getListCollectionsQueryOptions());
    const counts = libraryCountsQueryOptions().map((options) => queryClient.ensureQueryData(options));
    const status = (await viewerIsAdmin(queryClient))
      ? queryClient.ensureQueryData(libraryApi.getLibraryStatusQueryOptions())
      : undefined;
    await Promise.all([collections, ...counts, status]);
  },
});

export { Route };
