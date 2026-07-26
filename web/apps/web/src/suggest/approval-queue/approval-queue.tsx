import { type ApprovalEditDTO, suggestionsApi } from "@loomarr/api";
import { useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { useState } from "react";
import { ApprovalQueueItem, EmptyState, ErrorState } from "@/components/loomarr";
import { Button, Checkbox } from "@/components/ui";

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

  // Pending edits, keyed by proposal id — several rows can be open at once and each carries its
  // own delta. A row with no entry (or an `undefined` entry) is unmodified.
  const [edits, setEdits] = useState<Record<string, ApprovalEditDTO | undefined>>({});
  const setEdit = (id: string, edit: ApprovalEditDTO | undefined) =>
    setEdits((prev) => ({ ...prev, [id]: edit }));

  // Bulk selection (V27), by proposal id.
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const toggleSelected = (id: string, on: boolean) =>
    setSelected((prev) => {
      const next = new Set(prev);
      if (on) next.add(id);
      else next.delete(id);
      return next;
    });

  // Bulk approve goes to its own endpoint, which loops the SINGLE approve handler server-side —
  // so the gate stays one implementation. Clearing the selection on success matters: the ids
  // that went through are no longer `submitted`, and a stale selection would offer to approve
  // them again.
  const bulk = suggestionsApi.useBulkApproveProposals({
    mutation: {
      onSuccess: () => {
        setSelected(new Set());
        invalidate();
      },
    },
  });

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

  const busy = approve.isPending || deny.isPending || bulk.isPending;

  // ⚠ A row with a PENDING EDIT is excluded from bulk. The bulk endpoint takes no edit field,
  // deliberately — "drop these two titles" means nothing applied across a batch — so bulk-
  // approving an edited row would silently discard the admin's edit and acquire the titles they
  // just removed. Those rows keep their own Approve button, which does carry the edit.
  const edited = new Set(
    Object.entries(edits)
      .filter(([, e]) => e !== undefined)
      .map(([id]) => id),
  );
  const bulkable = rows.filter((p) => !edited.has(p.id));
  const selectedBulkable = bulkable.filter((p) => selected.has(p.id));

  return (
    <div className="flex flex-col gap-3">
      {(approve.error ?? deny.error ?? bulk.error) != null && (
        <ErrorState error={approve.error ?? deny.error ?? bulk.error} />
      )}

      {/* Bulk approve (V27). Shown only when there is more than one approvable row — a "select
          all" above a single item is noise. */}
      {bulkable.length > 1 && (
        <div className="flex flex-wrap items-center gap-3 rounded-md border border-border bg-card px-3 py-2">
          <label className="flex cursor-pointer items-center gap-2 text-sm">
            <Checkbox
              checked={selectedBulkable.length === bulkable.length && bulkable.length > 0}
              disabled={busy}
              aria-label="Select all for bulk approve"
              onChange={(e) => setSelected(e.target.checked ? new Set(bulkable.map((p) => p.id)) : new Set())}
            />
            Select all
          </label>
          <span className="text-muted-foreground text-sm">
            {selectedBulkable.length > 0 ? `${selectedBulkable.length} selected` : "None selected"}
          </span>
          {edited.size > 0 && (
            <span className="text-caution text-xs">
              {`${edited.size} edited ${edited.size === 1 ? "request is" : "requests are"} excluded — approve those individually so the edit is kept.`}
            </span>
          )}
          <Button
            size="sm"
            className="ml-auto"
            disabled={busy || selectedBulkable.length === 0}
            onClick={() => bulk.mutate({ data: { ids: selectedBulkable.map((p) => p.id) } })}
          >
            {`Approve ${selectedBulkable.length || ""}`.trim()}
          </Button>
        </div>
      )}

      <ul className="flex flex-col gap-2">
        {rows.map((p) => (
          <li key={p.id} className="flex items-start gap-2">
            {/* Only offered on rows bulk can actually take. An edited row has no checkbox at
                all rather than a disabled one: the row's own Approve button is the correct
                action for it, and a greyed box invites a click that would do the wrong thing. */}
            {bulkable.length > 1 && !edited.has(p.id) && (
              <Checkbox
                className="mt-4"
                checked={selected.has(p.id)}
                disabled={busy}
                aria-label={`Select ${p.proposal.intent?.description ?? "request"} for bulk approve`}
                onChange={(e) => toggleSelected(p.id, e.target.checked)}
              />
            )}
            <ApprovalQueueItem
              className="flex-1"
              title={p.proposal.intent?.description ?? "Suggested lineup"}
              requestedBy={p.createdBy}
              summary={p.proposal.rationale}
              acquisitions={p.proposal.acquisitions?.length ?? 0}
              lineup={p.proposal.lineup ?? []}
              acquisitionItems={p.proposal.acquisitions ?? []}
              status={busy ? "approving" : "pending"}
              onEdit={(edit) => setEdit(p.id, edit)}
              // The edit rides the SAME approve call — there is no separate "save edit" step,
              // because the edit is a parameter to the one approval gate, not a mutation of the
              // proposal (§7 / D-K). An unmodified row sends `{}`, exactly as it always has:
              // the handler maps a body with no drops, adds or note to a nil edit, so approving
              // untouched stays byte-identical to the pre-V25 behaviour.
              onApprove={() => approve.mutate({ id: p.id, data: edits[p.id] ?? {} })}
              onDeny={(reason) => deny.mutate({ id: p.id, data: { reason } })}
            />
          </li>
        ))}
      </ul>
    </div>
  );
};

export { ApprovalQueue };
