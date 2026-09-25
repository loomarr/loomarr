import { createFileRoute, redirect } from "@tanstack/react-router";
import { meQueryOptions } from "@/auth/me-query";
import { ApprovalHistory } from "@/queue/approval-history";

// History — a record of other people's decisions is not a member's to read (§11). The layout
// only offers this tab's link to an admin, and `beforeLoad` below sends a member who types the
// path to In flight (the server refuses them the list too). `ApprovalHistory` carries no role
// gate of its own.
const HistoryScreen = () => <ApprovalHistory />;

const Route = createFileRoute("/_authed/queue/history")({
  // Admin-only (§11) — enforced here, not just by hiding the tab link: a member who types the path
  // is sent to the one tab they have rather than shown a list whose every action would 403.
  beforeLoad: async ({ context }) => {
    const me = await context.queryClient.ensureQueryData(meQueryOptions());
    if (me.status !== 200 || me.data.role !== "admin") throw redirect({ to: "/queue/flight" });
  },
  component: HistoryScreen,
});

export { Route };
