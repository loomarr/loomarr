import * as libraryApi from "@loomarr/api/endpoints/library";
import { ListLibraryItemsHdr } from "@loomarr/api/models/listLibraryItemsHdr";
import type { LibraryKind } from "@loomarr/ui";
import { createFileRoute } from "@tanstack/react-router";
import { viewerIsAdmin } from "@/auth/me-query";
import { useDocumentTitle } from "@/lib/use-document-title";
import { libraryCountsQueryOptions, libraryItemsQueryOptions } from "@/library/library-items-query";
import { LibraryPage } from "@/library/library-page";

// The wall (#4476): an admin's, with the status line, and a member's, read-only. The server
// decides what each reads (a member's titles leave out files and fixes) and refuses a member every
// other /v1/library route. The filter, the order and the search are query params so a view
// survives Back and a reload; each is narrowed here so a hand-typed link can't push garbage into
// the list query.

interface LibrarySearch {
  kind?: LibraryKind;
  q?: string;
  /** Recently added first; absent is by name. */
  sort?: "recent";
  genre?: string;
  studio?: string;
  person?: string;
  audio?: string;
  subtitles?: string;
  uhd?: boolean;
  hdr?: ListLibraryItemsHdr;
}

// A text filter, cut to what the server takes. The router reads "1917" in a URL as a number.
const text = <K extends string>(search: Record<string, unknown>, key: K, max: number) => {
  const raw = search[key];
  const value = typeof raw === "number" ? String(raw) : raw;
  return typeof value === "string" && value.trim() !== ""
    ? ({ [key]: value.slice(0, max) } as Record<K, string>)
    : {};
};

const HDR_KINDS: readonly unknown[] = Object.values(ListLibraryItemsHdr);

const validateLibrarySearch = (search: Record<string, unknown>): LibrarySearch => ({
  ...(search.kind === "movie" || search.kind === "series" ? { kind: search.kind } : {}),
  ...(typeof search.q === "string" && search.q.trim() !== "" ? { q: search.q.slice(0, 200) } : {}),
  ...(search.sort === "recent" ? { sort: search.sort } : {}),
  ...text(search, "genre", 100),
  ...text(search, "studio", 200),
  ...text(search, "person", 600),
  ...text(search, "audio", 35),
  ...text(search, "subtitles", 35),
  ...(search.uhd === true || search.uhd === "true" ? { uhd: true } : {}),
  ...(HDR_KINDS.includes(search.hdr) ? { hdr: search.hdr as ListLibraryItemsHdr } : {}),
});

const LibraryScreen = () => {
  useDocumentTitle("Library");
  const { kind, q, sort, genre, ...filters } = Route.useSearch();
  return (
    <LibraryPage
      filter={kind ?? "all"}
      filters={filters}
      genre={genre}
      search={q ?? ""}
      sort={sort ?? "name"}
    />
  );
};

const Route = createFileRoute("/_authed/library/")({
  validateSearch: validateLibrarySearch,
  loaderDeps: ({ search }) => search,
  // What the wall paints: the first page of titles, the tabs' counts, and for an admin the status
  // line over them. The page's search box starts from the URL's `q`, trimmed, so that is the entry
  // warmed here. A member's status request would 403, so only an admin's is asked.
  loader: async ({ context: { queryClient }, deps: { q, ...filter } }) => {
    const titles = queryClient.ensureInfiniteQueryData(
      libraryItemsQueryOptions({ ...filter, search: q?.trim() || undefined }),
    );
    const counts = libraryCountsQueryOptions().map((options) => queryClient.ensureQueryData(options));
    const status = (await viewerIsAdmin(queryClient))
      ? queryClient.ensureQueryData(libraryApi.getLibraryStatusQueryOptions())
      : undefined;
    await Promise.all([titles, ...counts, status]);
  },
  component: LibraryScreen,
});

export { Route };
