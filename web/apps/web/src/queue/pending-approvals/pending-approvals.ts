import * as fillerApi from "@loomarr/api/endpoints/filler";
import * as proposalsApi from "@loomarr/api/endpoints/proposals";
import { unwrap } from "@loomarr/api/unwrap";

// What is waiting on an admin's yes: submitted channel proposals AND pending filler pulls. Both
// are decisions the Approval tab lists, so the tab count, the Dashboard's "Needs you" card and
// the Approval list all read THIS — counting proposals alone made a pending pull invisible
// everywhere but a list that was hidden whenever there were no proposals (#1404).
//
// Gated by `enabled` (the caller's isAdmin): approving is admin-only (§11), so a member's page
// load must not fire queries that can only 403.
const usePendingApprovals = (enabled: boolean) => {
  const proposals = proposalsApi.useListProposals(
    { status: "submitted" },
    { query: { enabled, retry: false } },
  );
  const pulls = fillerApi.useListFillerPulls({ status: "pending" }, { query: { enabled, retry: false } });
  const proposalCount = unwrap(proposals.data, (b) => b.proposals?.length) ?? 0;
  const pullCount = unwrap(pulls.data, (b) => b.pulls?.length) ?? 0;
  return { count: proposalCount + pullCount, proposalCount, pullCount };
};

export { usePendingApprovals };
