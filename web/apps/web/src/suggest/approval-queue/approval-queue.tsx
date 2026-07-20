import { suggestionsApi } from "@loomarr/api";
import { useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
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

  // Approving CREATES the channel (§7), so it ends by showing the operator the thing
  // they just made rather than an emptier queue. Without this the flow stops at
  // "approved" and the channel — the entire point — is somewhere they have to go find.
  //
  // Guarded on channelId: approval is durable even when channel creation fails, and in
  // that case there is nowhere to navigate to. Staying put is right, and the queue
  // refresh still shows the proposal left the queue.
  const navigate = useNavigate();
  const approve = suggestionsApi.useApproveProposal({
    mutation: {
      onSuccess: (res) => {
        invalidate();
        const channelId = res.status === 200 ? res.data.channelId : undefined;
        if (channelId) void navigate({ to: "/channels/$id", params: { id: channelId } });
      },
    },
  });
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
