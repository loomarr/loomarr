import { proposalsApi, unwrap } from "@loomarr/api";
import { useQueries } from "@tanstack/react-query";
import { ApprovalHistoryRow, EmptyState, ErrorState } from "@/components/loomarr";

// ApprovalHistory — Queue's History tab (V27): every DECIDED proposal, newest first.
//
// This is the audit trail for the approval gate. Approving is the only path from a proposal to a
// `wanted` title, so "what did we let through, when, and did anyone change it before letting it
// through" is a question about the gate itself — and until V26/V27 none of the answer was
// reachable: `approvedBy` was stored and rendered nowhere, `modSummary`/`note` never left the
// server, and `approvedAt` did not exist.
//
// Approved and denied are fetched separately because `GET /v1/proposals` filters by ONE status
// per call, then merged and sorted here.
const ApprovalHistory = () => {
  const queries = useQueries({
    queries: (["approved", "denied"] as const).map((status) =>
      proposalsApi.getListProposalsQueryOptions({ status }),
    ),
  });

  const error = queries.find((q) => q.error)?.error;
  const rows = queries
    .flatMap((q) => unwrap(q.data, (b) => b.proposals) ?? [])
    // Newest decision first. A denial carries no `approvedAt`, so it sorts by empty string and
    // lands at the end — acceptable while nothing records a decided-at for denials, and better
    // than inventing a timestamp for it.
    .sort((a, b) => (b.approvedAt ?? "").localeCompare(a.approvedAt ?? ""));

  if (error != null) return <ErrorState error={error} />;

  if (rows.length === 0) {
    return (
      <EmptyState
        title="Nothing decided yet"
        description="Once requests are approved or declined, they're recorded here: who decided, when, and what they changed."
      />
    );
  }

  return (
    <ul className="flex flex-col gap-2">
      {rows.map((p) => (
        <ApprovalHistoryRow key={p.id} proposal={p} />
      ))}
    </ul>
  );
};

export { ApprovalHistory };
