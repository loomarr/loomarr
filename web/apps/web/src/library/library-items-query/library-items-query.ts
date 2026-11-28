import * as libraryApi from "@loomarr/api/endpoints/library";
import type { ListLibraryItemsHdr } from "@loomarr/api/models/listLibraryItemsHdr";
import type { ListLibraryItemsKind } from "@loomarr/api/models/listLibraryItemsKind";
import type { ListLibraryItemsSort } from "@loomarr/api/models/listLibraryItemsSort";
import { unwrap } from "@loomarr/api/unwrap";
import { infiniteQueryOptions } from "@tanstack/react-query";

// The Library grid's titles, a page at a time. The page reads it with useInfiniteQuery and the
// Library route's loader ensures its first page, so both name ONE query: the key (the list's
// generated key plus "pages", so an invalidation of `/v1/library/items` reaches it) and the shape
// of a page live here, and a loader that built its own would warm an entry nobody reads.
const LIBRARY_PAGE_SIZE = 60;
// The most the server returns in one page.
const LIBRARY_PAGE_MAX = 200;

interface LibraryItemsFilter {
  kind?: ListLibraryItemsKind;
  /** The search as the server is asked for it: already settled and trimmed, absent when empty. */
  search?: string;
  /** Recently added first; absent is the server's order, by name. */
  sort?: typeof ListLibraryItemsSort.recent;
  /** Only titles with this genre. */
  genre?: string;
  studio?: string;
  /** Only titles crediting this person, by id. */
  person?: string;
  audio?: string;
  subtitles?: string;
  uhd?: boolean;
  hdr?: ListLibraryItemsHdr;
  /**
   * How far down the list a jump wants loaded: pages short of it ask for as much as the server
   * gives at once. It is not part of the key, since the titles are the same list however they
   * are paged.
   */
  reach?: () => number;
}

const libraryItemsQueryOptions = ({ reach, ...filter }: LibraryItemsFilter) => {
  const params = { ...filter, limit: LIBRARY_PAGE_SIZE };
  return infiniteQueryOptions({
    queryKey: [...libraryApi.getListLibraryItemsQueryKey(params), "pages"],
    initialPageParam: 0,
    queryFn: async ({ client, queryKey, pageParam, signal }) => {
      // A refetch asks for each page again by count, so a page asks for as many titles as the
      // cache holds from its offset: otherwise the large pages a jump loaded come back small and
      // the list shrinks under the reader.
      const cached = client.getQueryData<{ pages: { items: unknown[] }[] }>(queryKey)?.pages ?? [];
      let start = 0;
      let held = 0;
      for (const page of cached) {
        if (start === pageParam) held = page.items.length;
        start += page.items.length;
      }
      const wanted = Math.max(held, (reach?.() ?? 0) - pageParam);
      const limit = Math.min(LIBRARY_PAGE_MAX, Math.max(LIBRARY_PAGE_SIZE, wanted));
      const res = await libraryApi.listLibraryItems({ ...params, limit, offset: pageParam }, { signal });
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

// The tabs' counts: one title of everything, of the movies and of the shows, read for the total.
// The page reads them and the Library's loaders warm them, so both name these params.
const LIBRARY_COUNTS = [
  { limit: 1 },
  { kind: "movie", limit: 1 },
  { kind: "series", limit: 1 },
] as const satisfies readonly { kind?: ListLibraryItemsKind; limit: number }[];

const libraryCountsQueryOptions = () =>
  LIBRARY_COUNTS.map((params) => libraryApi.getListLibraryItemsQueryOptions(params));

export type { LibraryItemsFilter };
export { LIBRARY_COUNTS, LIBRARY_PAGE_SIZE, libraryCountsQueryOptions, libraryItemsQueryOptions };
