import { createFileRoute, redirect } from "@tanstack/react-router";
import { meQueryOptions } from "@/auth/me-query";
import { ApprovalQueue } from "@/suggest/approval-queue";

// Needs approval — admin-only (§11): the layout (`route.tsx`) only offers this tab's link to an
// admin, and `beforeLoad` below sends a member who types the path to In flight.
const ApprovalScreen = () => <ApprovalQueue />;

const Route = createFileRoute("/_authed/queue/approval")({
  // Admin-only (§11) — enforced here, not just by hiding the tab link: a member who types the path
  // is sent to the one tab they have rather than shown a list whose every action would 403.
  beforeLoad: async ({ context }) => {
    const me = await context.queryClient.ensureQueryData(meQueryOptions());
    if (me.status !== 200 || me.data.role !== "admin") throw redirect({ to: "/queue/flight" });
  },
  component: ApprovalScreen,
});

export { Route };
