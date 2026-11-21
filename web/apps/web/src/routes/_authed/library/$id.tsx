import * as libraryApi from "@loomarr/api/endpoints/library";
import { createFileRoute } from "@tanstack/react-router";
import { AdminOnly } from "@/auth/admin-only";
import { viewerIsAdmin } from "@/auth/me-query";
import { LibraryTitlePage } from "@/library/library-title-page";

const LibraryTitleScreen = () => {
  const { id } = Route.useParams();
  return (
    <AdminOnly>
      <LibraryTitlePage id={id} />
    </AdminOnly>
  );
};

const Route = createFileRoute("/_authed/library/$id")({
  component: LibraryTitleScreen,
  // The title's detail is the whole page. Admin-only like the page.
  loader: async ({ context: { queryClient }, params: { id } }) => {
    if (!(await viewerIsAdmin(queryClient))) return;
    await queryClient.ensureQueryData(libraryApi.getGetLibraryItemQueryOptions(id));
  },
});

export { Route };
