import * as fillerApi from "@loomarr/api/endpoints/filler";
import { createFileRoute } from "@tanstack/react-router";
import { openFillerPage } from "@/filler/filler-loader";
import { FillerPage } from "@/filler/filler-page";

// Taxonomy is a first-class library destination: everyone who can browse filler can understand
// its vocabulary and coverage; the mutation controls inside remain admin-only and are enforced
// again by the API.
const TaxonomyScreen = () => <FillerPage tab="taxonomy" />;

const Route = createFileRoute("/_authed/filler/taxonomy")({
  // The vocabulary the tab is drawn from, so it opens on it and a failed read is the route's error
  // view. Member-readable, like the page.
  loader: async ({ context: { queryClient } }) => {
    await Promise.all([
      openFillerPage(queryClient),
      queryClient.ensureQueryData(fillerApi.getListTaxonomyQueryOptions()),
    ]);
  },
  component: TaxonomyScreen,
});

export { Route };
