import * as libraryApi from "@loomarr/api/endpoints/library";
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
}

const validateLibrarySearch = (search: Record<string, unknown>): LibrarySearch => ({
  ...(search.kind === "movie" || search.kind === "series" ? { kind: search.kind } : {}),
  ...(typeof search.q === "string" && search.q.trim() !== "" ? { q: search.q.slice(0, 200) } : {}),
  ...(search.sort === "recent" ? { sort: search.sort } : {}),
});

const LibraryScreen = () => {
  useDocumentTitle("Library");
  const { kind, q, sort } = Route.useSearch();
  return <LibraryPage filter={kind ?? "all"} search={q ?? ""} sort={sort ?? "name"} />;
};

const Route = createFileRoute("/_authed/library/")({
  validateSearch: validateLibrarySearch,
  loaderDeps: ({ search: { kind, q, sort } }) => ({ kind, q, sort }),
  // What the wall paints: the first page of titles, the tabs' counts, and for an admin the status
  // line over them. The page's search box starts from the URL's `q`, trimmed, so that is the entry
  // warmed here. A member's status request would 403, so only an admin's is asked.
  loader: async ({ context: { queryClient }, deps: { kind, q, sort } }) => {
    const titles = queryClient.ensureInfiniteQueryData(
      libraryItemsQueryOptions({ kind, search: q?.trim() || undefined, sort }),
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
