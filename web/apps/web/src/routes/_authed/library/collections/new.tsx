import { createFileRoute } from "@tanstack/react-router";
import { AdminOnly } from "@/auth/admin-only";
import { useDocumentTitle } from "@/lib/use-document-title";
import { CollectionEditorPage } from "@/library/collection-editor-page";

// A new household collection (#2151 decision 28): the editor, empty. Admin-only.

const NewCollectionScreen = () => {
  useDocumentTitle("New collection");
  return (
    <AdminOnly>
      <CollectionEditorPage />
    </AdminOnly>
  );
};

// no-loader: a new collection has nothing saved to read
const Route = createFileRoute("/_authed/library/collections/new")({
  component: NewCollectionScreen,
});

export { Route };
