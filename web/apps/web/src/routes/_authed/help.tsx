import { createFileRoute } from "@tanstack/react-router";
import { HelpPage } from "@/help";

// `?page=` is validated rather than trusted: the API emits `troubleshooting#tunarr`-style
// deep-links, and typed search params mean a link into Help is as type-safe as any other
// route. An absent or non-string value falls back to the first page.
const Route = createFileRoute("/_authed/help")({
  validateSearch: (search: Record<string, unknown>): { page?: string } =>
    typeof search.page === "string" ? { page: search.page } : {},
  component: HelpPage,
});

export { Route };
