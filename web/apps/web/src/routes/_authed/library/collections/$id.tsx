import * as libraryApi from "@loomarr/api/endpoints/library";
import { createFileRoute } from "@tanstack/react-router";
import { AdminOnly } from "@/auth/admin-only";
import { viewerIsAdmin } from "@/auth/me-query";
import { useDocumentTitle } from "@/lib/use-document-title";
import { CollectionEditorPage } from "@/library/collection-editor-page";

// A household collection's editor (#2151 decision 28). Admin-only like the rest of the Library.

const CollectionScreen = () => {
  useDocumentTitle("Collection");
  const { id } = Route.useParams();
  return (
    <AdminOnly>
      <CollectionEditorPage id={id} key={id} />
    </AdminOnly>
  );
};

const Route = createFileRoute("/_authed/library/collections/$id")({
  component: CollectionScreen,
  // The saved collection the editor opens on. Admin-only like the page.
  loader: async ({ context: { queryClient }, params: { id } }) => {
    if (!(await viewerIsAdmin(queryClient))) return;
    await queryClient.ensureQueryData(libraryApi.getGetCollectionQueryOptions(id));
  },
});

export { Route };
