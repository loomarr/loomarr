import * as libraryApi from "@loomarr/api/endpoints/library";
import type { ListLibraryItemsKind } from "@loomarr/api/models/listLibraryItemsKind";
import { unwrap } from "@loomarr/api/unwrap";
import { infiniteQueryOptions } from "@tanstack/react-query";

// The Library grid's titles, a page at a time. The page reads it with useInfiniteQuery and the
// Library route's loader ensures its first page, so both name ONE query: the key (the list's
// generated key plus "pages", so an invalidation of `/v1/library/items` reaches it) and the shape
// of a page live here, and a loader that built its own would warm an entry nobody reads.
const LIBRARY_PAGE_SIZE = 60;

interface LibraryItemsFilter {
  kind?: ListLibraryItemsKind;
  /** The search as the server is asked for it: already settled and trimmed, absent when empty. */
  search?: string;
}

const libraryItemsQueryOptions = ({ kind, search }: LibraryItemsFilter) => {
  const params = { kind, search, limit: LIBRARY_PAGE_SIZE };
  return infiniteQueryOptions({
    queryKey: [...libraryApi.getListLibraryItemsQueryKey(params), "pages"],
    initialPageParam: 0,
    queryFn: async ({ pageParam, signal }) => {
      const res = await libraryApi.listLibraryItems({ ...params, offset: pageParam }, { signal });
      const body = unwrap(res);
      if (!body) throw new Error("Couldn't list the library");
      return body;
    },
    getNextPageParam: (last, pages) => {
      const loaded = pages.reduce((n, page) => n + page.items.length, 0);
      return loaded < last.total && last.items.length > 0 ? loaded : undefined;
    },
  });
};

export type { LibraryItemsFilter };
export { libraryItemsQueryOptions };
