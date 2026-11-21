import * as helpApi from "@loomarr/api/endpoints/help";
import { createFileRoute } from "@tanstack/react-router";
import { HelpPage } from "@/help/help-page";

// `?page=` is validated rather than trusted: the API emits `troubleshooting#tunarr`-style
// deep-links, and typed search params mean a link into Help is as type-safe as any other
// route. An absent or non-string value falls back to the first page.
// `page` selects the doc; `section` scrolls to a heading within it. Both are typed so a
// deep-link ("troubleshooting#tunarr", parsed into these two) is as type-safe as any
// route, and a red check can land the operator on the exact section (§13).
const Route = createFileRoute("/_authed/help")({
  validateSearch: (search: Record<string, unknown>): { page?: string; section?: string } => ({
    ...(typeof search.page === "string" ? { page: search.page } : {}),
    ...(typeof search.section === "string" ? { section: search.section } : {}),
  }),
  // The page list and the document the page opens on: the one the link names, else the first (what
  // the page itself falls back to). The page shows its own state for each, so they are warmed.
  loaderDeps: ({ search: { page, section } }) => ({ page, section }),
  loader: async ({ context: { queryClient }, deps: { page, section } }) => {
    const list = await queryClient.fetchQuery(helpApi.getListDocsQueryOptions()).catch(() => undefined);
    const active = page ?? (list?.status === 200 ? list.data.docs?.[0]?.slug : undefined);
    if (!active) return;
    await queryClient.prefetchQuery(helpApi.getGetDocQueryOptions(active, section ? { section } : undefined));
  },
  component: HelpPage,
});

export { Route };
