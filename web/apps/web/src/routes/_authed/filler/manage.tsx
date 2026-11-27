import * as fillerApi from "@loomarr/api/endpoints/filler";
import { createFileRoute } from "@tanstack/react-router";
import { openFillerPage, warmDefectReport } from "@/filler/filler-loader";
import { FillerPage } from "@/filler/filler-page";

const ManageScreen = () => <FillerPage tab="manage" />;

const Route = createFileRoute("/_authed/filler/manage")({
  // The Manage rows are links that stand without their summaries, so the tag counts and, for an
  // admin, the quality count are warmed.
  loader: async ({ context: { queryClient } }) => {
    await Promise.all([
      openFillerPage(queryClient),
      queryClient.prefetchQuery(fillerApi.getListTaxonomyQueryOptions()),
      warmDefectReport(queryClient),
    ]);
  },
  component: ManageScreen,
});

export { Route };
