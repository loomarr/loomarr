import { suggestionsApi } from "@loomarr/api";
import { useQueryClient } from "@tanstack/react-query";
import { ApprovalQueueItem, EmptyState, ErrorState } from "@/components/loomarr";

// The admin approval queue (§7, §11) — the human gate every acquisition passes through.
// It lists everything still `submitted`, which is exactly what the status filter means.
// Approving is the ONLY path from a proposal to /v1/titles, so this list is the audit
// surface for "what is about to spend real resources".
const ApprovalQueue = () => {
  const queryClient = useQueryClient();
  const proposals = suggestionsApi.useListProposals({ status: "submitted" }, { query: { retry: false } });
  const invalidate = () =>
    queryClient.invalidateQueries({ queryKey: suggestionsApi.getListProposalsQueryKey() });

  const approve = suggestionsApi.useApproveProposal({ mutation: { onSuccess: invalidate } });
  const deny = suggestionsApi.useDenyProposal({ mutation: { onSuccess: invalidate } });

  if (proposals.error) {
    return <ErrorState error={proposals.error} onRetry={() => proposals.refetch()} />;
  }

  const rows = proposals.data?.status === 200 ? (proposals.data.data.proposals ?? []) : [];

  if (rows.length === 0) {
    return (
      <EmptyState
        title="Queue's clear"
        description="Nothing is waiting for approval. Suggested lineups land here before anything is acquired."
      />
    );
  }

  const busy = approve.isPending || deny.isPending;

  return (
    <div className="flex flex-col gap-3">
      {(approve.error ?? deny.error) != null && <ErrorState error={approve.error ?? deny.error} />}
      <ul className="flex flex-col gap-2">
        {rows.map((p) => (
          <li key={p.id}>
            <ApprovalQueueItem
              title={p.proposal.intent?.description ?? "Suggested lineup"}
              requestedBy={p.createdBy}
              summary={p.proposal.rationale}
              acquisitions={p.proposal.acquisitions?.length ?? 0}
              status={busy ? "approving" : "pending"}
              onApprove={() => approve.mutate({ id: p.id })}
              onDeny={() => deny.mutate({ id: p.id, data: {} })}
            />
          </li>
        ))}
      </ul>
    </div>
  );
};

export { ApprovalQueue };
