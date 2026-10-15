import { createFileRoute, Outlet, useParams } from "@tanstack/react-router";
import { AdminOnly } from "@/auth/admin-only";
import { FillerPage } from "@/filler/filler-page";

// Admin-only: a member gets AdminOnly's no-access state, and the sources page never mounts.
const FillerSourcesLayout = () => {
  const { sourceId } = useParams({ strict: false });
  return (
    <AdminOnly>
      <FillerPage tab="sources" sourceID={sourceId} />
      <Outlet />
    </AdminOnly>
  );
};

const Route = createFileRoute("/_authed/filler/sources")({
  component: FillerSourcesLayout,
});

export { Route };
