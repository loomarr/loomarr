import { createFileRoute } from "@tanstack/react-router";
import { FillerPage } from "@/filler/filler-page";

// Taxonomy is a first-class library destination: everyone who can browse filler can understand
// its vocabulary and coverage; the mutation controls inside remain admin-only and are enforced
// again by the API.
const TaxonomyScreen = () => <FillerPage tab="taxonomy" />;

const Route = createFileRoute("/_authed/filler/taxonomy")({
  component: TaxonomyScreen,
});

export { Route };
