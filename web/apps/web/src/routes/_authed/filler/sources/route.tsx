import * as fillerApi from "@loomarr/api/endpoints/filler";
import { createFileRoute, Outlet, redirect, useParams } from "@tanstack/react-router";
import { meQueryOptions } from "@/auth/me-query";
import { openFillerPage } from "@/filler/filler-loader";
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
  // Admin-only (beforeLoad has sent a member away). The sources the tab lists, so it opens on them
  // and a failed read is the route's error view.
  loader: async ({ context: { queryClient } }) => {
    await Promise.all([
      openFillerPage(queryClient),
      queryClient.ensureQueryData(fillerApi.getListFillerSourcesQueryOptions()),
    ]);
  },
  component: FillerSourcesLayout,
});

export { Route };
