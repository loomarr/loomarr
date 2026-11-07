import type { LibraryKind } from "@loomarr/ui";
import { createFileRoute } from "@tanstack/react-router";
import { AdminOnly } from "@/auth/admin-only";
import { useDocumentTitle } from "@/lib/use-document-title";
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
  component: LibraryScreen,
});

export { Route };
