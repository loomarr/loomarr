import { createFileRoute, redirect } from "@tanstack/react-router";

// Incoming — what has arrived but isn't filed yet. Its own path (V-nav-paths), same as
// Catalog and Sources. ⚠ Carries NONE of the catalog's filters: `q`, `kind` and `audience`
// narrow the clip grid, and nothing on this tab is a clip — a search term dragged over from
// Catalog would leave a filter applied to a list it cannot narrow (the same reasoning the
// old `catalogSearch` carve-out in filler-page recorded).
const Route = createFileRoute("/_authed/filler/incoming")({
  beforeLoad: () => {
    throw redirect({ to: "/filler/attention" });
  },
});

export { Route };
