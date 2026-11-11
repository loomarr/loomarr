import * as fillerApi from "@loomarr/api/endpoints/filler";
import { createFileRoute } from "@tanstack/react-router";
import { warmDefectReport, warmFillerPage } from "@/filler/filler-loader";
import { FillerPage } from "@/filler/filler-page";

const ManageScreen = () => <FillerPage tab="manage" />;

const Route = createFileRoute("/_authed/filler/manage")({
  // The summaries the Manage rows are made of: the tag counts and, for an admin, the quality count.
  loader: async ({ context: { queryClient } }) => {
    await Promise.all([
      warmFillerPage(queryClient),
      queryClient.prefetchQuery(fillerApi.getListTaxonomyQueryOptions()),
      warmDefectReport(queryClient),
    ]);
  },
  component: ManageScreen,
});

export { Route };
