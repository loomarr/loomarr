import * as libraryApi from "@loomarr/api/endpoints/library";
import type { LibraryKind } from "@loomarr/ui";
import { createFileRoute } from "@tanstack/react-router";
import { AdminOnly } from "@/auth/admin-only";
import { viewerIsAdmin } from "@/auth/me-query";
import { useDocumentTitle } from "@/lib/use-document-title";
import { libraryItemsQueryOptions } from "@/library/library-items-query";
import { LibraryPage } from "@/library/library-page";

// The Library is admin-only (#2151 decision 22); the server enforces it on every /v1/library
// route. Its filter and search are query params so a filtered view survives Back and a reload;
// each is narrowed here so a hand-typed link can't push garbage into the list query.

interface LibrarySearch {
  kind?: LibraryKind;
  q?: string;
}

const validateLibrarySearch = (search: Record<string, unknown>): LibrarySearch => ({
  ...(search.kind === "movie" || search.kind === "series" ? { kind: search.kind } : {}),
  ...(typeof search.q === "string" && search.q.trim() !== "" ? { q: search.q.slice(0, 200) } : {}),
});

const LibraryScreen = () => {
  useDocumentTitle("Library");
  const { kind, q } = Route.useSearch();
  return (
    <AdminOnly>
      <LibraryPage filter={kind ?? "all"} search={q ?? ""} />
    </AdminOnly>
  );
};

const Route = createFileRoute("/_authed/library/")({
  validateSearch: validateLibrarySearch,
  loaderDeps: ({ search: { kind, q } }) => ({ kind, q }),
  // What the Library paints: the status line, the Needs you strip, and the first page of titles.
  // The page's search box starts from the URL's `q`, trimmed, so that is the entry warmed here.
  // Admin-only like the page: a member's request would 403, and the page answers them itself.
  loader: async ({ context: { queryClient }, deps: { kind, q } }) => {
    if (!(await viewerIsAdmin(queryClient))) return;
    await Promise.all([
      queryClient.ensureQueryData(libraryApi.getLibraryStatusQueryOptions()),
      queryClient.ensureQueryData(libraryApi.getListLibraryNeedsQueryOptions()),
      queryClient.ensureInfiniteQueryData(libraryItemsQueryOptions({ kind, search: q?.trim() || undefined })),
    ]);
  },
  component: LibraryScreen,
});

export { Route };
