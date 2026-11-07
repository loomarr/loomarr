import { createFileRoute } from "@tanstack/react-router";
import { AdminOnly } from "@/auth/admin-only";
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
});

export { Route };
