import * as fillerApi from "@loomarr/api/endpoints/filler";
import { createFileRoute } from "@tanstack/react-router";
import { warmFillerPage } from "@/filler/filler-loader";
import { FillerPage } from "@/filler/filler-page";
import { catalogListParams, validateCatalogSearch } from "@/filler/filler-search";

const LibraryScreen = () => <FillerPage tab="library" />;

const Route = createFileRoute("/_authed/filler/library")({
  validateSearch: validateCatalogSearch,
  // The page of clips the filters ask for (the catalog's own query, through `catalogListParams`)
  // and the pool health line. Each shows its own state when it fails, so they are warmed.
  loaderDeps: ({ search }) => search,
  loader: async ({ context: { queryClient }, deps }) => {
    await Promise.all([
      warmFillerPage(queryClient),
      queryClient.prefetchQuery(fillerApi.getFillerPoolQueryOptions()),
      queryClient.prefetchQuery(fillerApi.getListFillerQueryOptions(catalogListParams(deps))),
    ]);
  },
  component: LibraryScreen,
});

export { Route };
