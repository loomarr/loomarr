import * as fillerApi from "@loomarr/api/endpoints/filler";
import { createFileRoute, redirect } from "@tanstack/react-router";
import { meQueryOptions } from "@/auth/me-query";
import { warmFillerPage } from "@/filler/filler-loader";
import { FillerPage } from "@/filler/filler-page";

// Incoming — what has arrived but isn't filed yet. Its own path (V-nav-paths), same as
// Catalog and Sources. ⚠ Carries NONE of the catalog's filters: `q`, `kind` and `audience`
// narrow the clip grid, and nothing on this tab is a clip — a search term dragged over from
// Catalog would leave a filter applied to a list it cannot narrow (the same reasoning the
// old `catalogSearch` carve-out in filler-page recorded).
const IncomingScreen = () => <FillerPage tab="incoming" />;

const Route = createFileRoute("/_authed/filler/incoming")({
  beforeLoad: async ({ context }) => {
    const me = await context.queryClient.ensureQueryData(meQueryOptions());
    if (me.status !== 200 || me.data.role !== "admin") {
      throw redirect({ to: "/filler/library" });
    }
  },
  // beforeLoad has already sent a member away, so only an admin gets here. What is on the way is
  // the page's own list and it shows its own error, so it is warmed beside the page's status line.
  loader: async ({ context: { queryClient } }) => {
    await Promise.all([
      warmFillerPage(queryClient),
      queryClient.prefetchQuery(fillerApi.getFillerIncomingQueryOptions()),
    ]);
  },
  component: IncomingScreen,
});

export { Route };
