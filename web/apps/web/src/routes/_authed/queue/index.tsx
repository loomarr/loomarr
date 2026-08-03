import { suggestionsApi } from "@loomarr/api";
import { createFileRoute, redirect } from "@tanstack/react-router";
import { meQueryOptions } from "@/auth";

// /queue opens on whichever tab has work: an admin with pending proposals lands on Approval,
// since that is the one queue state that blocks somebody else. Everyone else lands on In
// flight — the titles they came to check on. Members only ever get In flight (approving is
// admin-only, §11).
//
// ⚠ Resolved in `beforeLoad`, not client-side after render: the parent `_authed` layout has
// already primed the `me` query (its own beforeLoad awaits it), so reading it here is free, and
// awaiting the pending-proposals count for an admin means the URL settles on the real
// destination before paint — no flash of "/queue/flight" that then jumps to "/queue/approval".
const Route = createFileRoute("/_authed/queue/")({
  beforeLoad: async ({ context }) => {
    const me = await context.queryClient.ensureQueryData(meQueryOptions());
    const isAdmin = me.status === 200 && me.data.role === "admin";
    if (!isAdmin) throw redirect({ to: "/queue/flight" });

    const proposals = await context.queryClient.ensureQueryData(
      suggestionsApi.getListProposalsQueryOptions({ status: "submitted" }),
    );
    const pendingCount = proposals.status === 200 ? (proposals.data.proposals?.length ?? 0) : 0;
    throw redirect({ to: pendingCount > 0 ? "/queue/approval" : "/queue/flight" });
  },
});

export { Route };
