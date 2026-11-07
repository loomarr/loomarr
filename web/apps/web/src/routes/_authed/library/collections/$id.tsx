import { createFileRoute } from "@tanstack/react-router";
import { AdminOnly } from "@/auth/admin-only";
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
});

export { Route };
