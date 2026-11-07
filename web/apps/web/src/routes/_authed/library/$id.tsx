import { createFileRoute } from "@tanstack/react-router";
import { AdminOnly } from "@/auth/admin-only";
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
});

export { Route };
