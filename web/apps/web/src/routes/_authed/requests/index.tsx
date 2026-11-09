import { createFileRoute, redirect } from "@tanstack/react-router";
import { meQueryOptions } from "@/auth/me-query";
import { fetchLandingTab } from "@/queue/use-requests";

// /requests opens on the first tab with something in it: Needs you, then In progress, then Done.
//
// ⚠ Resolved in `beforeLoad`, not after render: the parent `_authed` layout has already primed
// `me`, and awaiting the count means the URL settles on its real destination before paint — no
// flash of one tab that then jumps to another.
const Route = createFileRoute("/_authed/requests/")({
  beforeLoad: async ({ context }) => {
    const me = await context.queryClient.ensureQueryData(meQueryOptions());
    const isAdmin = me.status === 200 && me.data.role === "admin";
    const tab = await fetchLandingTab(context.queryClient, isAdmin);
    throw redirect({ to: `/requests/${tab}` });
  },
});

export { Route };
