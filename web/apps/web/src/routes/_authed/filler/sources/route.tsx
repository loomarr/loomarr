import * as fillerApi from "@loomarr/api/endpoints/filler";
import { createFileRoute, Outlet, redirect, useParams } from "@tanstack/react-router";
import { meQueryOptions } from "@/auth/me-query";
import { warmFillerPage } from "@/filler/filler-loader";
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
  // Admin-only (beforeLoad has sent a member away). The sources the tab lists, beside the page's
  // status line; a failed list is shown by the tab itself.
  loader: async ({ context: { queryClient } }) => {
    await Promise.all([
      warmFillerPage(queryClient),
      queryClient.prefetchQuery(fillerApi.getListFillerSourcesQueryOptions()),
    ]);
  },
  component: FillerSourcesLayout,
});

export { Route };
