import * as fillerApi from "@loomarr/api/endpoints/filler";
import { createFileRoute } from "@tanstack/react-router";
import { warmFillerPage } from "@/filler/filler-loader";
import { FillerPage } from "@/filler/filler-page";

// Taxonomy is a first-class library destination: everyone who can browse filler can understand
// its vocabulary and coverage; the mutation controls inside remain admin-only and are enforced
// again by the API.
const TaxonomyScreen = () => <FillerPage tab="taxonomy" />;

const Route = createFileRoute("/_authed/filler/taxonomy")({
  // The vocabulary the tab is drawn from, which it otherwise waits on ("Loading the taxonomy…").
  loader: async ({ context: { queryClient } }) => {
    await Promise.all([
      warmFillerPage(queryClient),
      queryClient.prefetchQuery(fillerApi.getListTaxonomyQueryOptions()),
    ]);
  },
  component: TaxonomyScreen,
});

export { Route };
