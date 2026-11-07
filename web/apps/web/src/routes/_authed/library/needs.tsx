import { createFileRoute } from "@tanstack/react-router";
import { AdminOnly } from "@/auth/admin-only";
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
});

export { Route };
