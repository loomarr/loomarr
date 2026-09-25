import { createFileRoute, redirect } from "@tanstack/react-router";
import { meQueryOptions } from "@/auth/me-query";
import { fetchNeedsYouCount } from "@/queue/use-requests";

// /requests opens on Needs you when something is waiting on the viewer, otherwise on In progress.
//
// ⚠ Resolved in `beforeLoad`, not after render: the parent `_authed` layout has already primed
// `me`, and awaiting the count means the URL settles on its real destination before paint — no
// flash of one tab that then jumps to another.
const Route = createFileRoute("/_authed/requests/")({
  beforeLoad: async ({ context }) => {
    const me = await context.queryClient.ensureQueryData(meQueryOptions());
    const isAdmin = me.status === 200 && me.data.role === "admin";
    const needsYou = await fetchNeedsYouCount(context.queryClient, isAdmin);
    throw redirect({ to: needsYou > 0 ? "/requests/needs-you" : "/requests/in-progress" });
  },
});

export { Route };
