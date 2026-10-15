import { createFileRoute, Outlet, redirect, useParams } from "@tanstack/react-router";
import { meQueryOptions } from "@/auth/me-query";
import { FillerPage } from "@/filler/filler-page";

const FillerSourcesLayout = () => {
  const { sourceId } = useParams({ strict: false });
  return (
    <>
      <FillerPage tab="sources" sourceID={sourceId} />
      <Outlet />
    </>
  );
};

const Route = createFileRoute("/_authed/filler/sources")({
  beforeLoad: async ({ context }) => {
    const me = await context.queryClient.ensureQueryData(meQueryOptions());
    if (me.status !== 200 || me.data.role !== "admin") {
      throw redirect({ to: "/filler/library" });
    }
  },
  component: FillerSourcesLayout,
});

export { Route };
