import { createFileRoute } from "@tanstack/react-router";
import { FillerPage } from "@/filler/filler-page";
import { validateCatalogSearch } from "@/filler/filler-search";

const LibraryScreen = () => <FillerPage tab="library" />;

const Route = createFileRoute("/_authed/filler/library")({
  validateSearch: validateCatalogSearch,
  component: LibraryScreen,
});

export { Route };
