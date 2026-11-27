import * as fillerApi from "@loomarr/api/endpoints/filler";
import { createFileRoute } from "@tanstack/react-router";
import { openFillerPage } from "@/filler/filler-loader";
import { FillerPage } from "@/filler/filler-page";
import { catalogListParams, validateCatalogSearch } from "@/filler/filler-search";

const LibraryScreen = () => <FillerPage tab="library" />;

const Route = createFileRoute("/_authed/filler/library")({
  validateSearch: validateCatalogSearch,
  // The page of clips the filters ask for (the catalog's own query, through `catalogListParams`)
  // and the pool health line. Both are warmed: the catalog answers for a failed read above its
  // toolbar, where changing the filters it came from is the way out, and the pool line is secondary.
  loaderDeps: ({ search }) => search,
  loader: async ({ context: { queryClient }, deps }) => {
    await Promise.all([
      openFillerPage(queryClient),
      queryClient.prefetchQuery(fillerApi.getFillerPoolQueryOptions()),
      queryClient.prefetchQuery(fillerApi.getListFillerQueryOptions(catalogListParams(deps))),
    ]);
  },
  component: LibraryScreen,
});

export { Route };
