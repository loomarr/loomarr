import { createFileRoute } from "@tanstack/react-router";
import { FillerPage } from "@/filler";

// Sources — where clips come from, and per-source search. Its own path (V-nav-paths), same
// as Catalog and Incoming. Carries none of the catalog's filters — see incoming.tsx.
const SourcesScreen = () => <FillerPage tab="sources" />;

const Route = createFileRoute("/_authed/filler/sources")({
  component: SourcesScreen,
});

export { Route };
