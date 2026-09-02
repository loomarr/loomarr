import * as fillerApi from "@loomarr/api/endpoints/filler";
import * as proposalsApi from "@loomarr/api/endpoints/proposals";
import type { ApprovalEditDTO } from "@loomarr/api/models/approvalEditDTO";
import { unwrap } from "@loomarr/api/unwrap";
import { provisionKey } from "@loomarr/core/provision";
import { useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { useState } from "react";
import { ApprovalQueueItem } from "@/components/loomarr/ai/approval-queue-item";
import { EmptyState } from "@/components/loomarr/feedback/empty-state";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { PullCard } from "@/components/loomarr/filler/pull-card";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { DiscoveryFeedbackControls, useDiscoveryFeedback } from "@/discovery-feedback";

// The admin approval queue (§7, §11) — the human gate every acquisition passes through.
// It lists everything still `submitted`, which is exactly what the status filter means.
// Approving is the ONLY path from a proposal to /v1/titles, so this list is the audit
// surface for "what is about to spend real resources".
const ApprovalQueue = () => {
  const queryClient = useQueryClient();
  const proposals = proposalsApi.useListProposals({ status: "submitted" }, { query: { retry: false } });
  const invalidate = () =>
    queryClient.invalidateQueries({ queryKey: proposalsApi.getListProposalsQueryKey() });

  // Approving CREATES the channel (§7), so it ends by showing the operator the thing
  // they just made rather than an emptier queue. Without this the flow stops at
  // "approved" and the channel — the entire point — is somewhere they have to go find.
  //
  // channelId is required on success because approval + titles + the local channel are one
  // transaction. A failure never removes the proposal from this queue.
  const navigate = useNavigate();
  const approve = proposalsApi.useApproveProposal({
    mutation: {
      onSuccess: (res) => {
        invalidate();
        if (res.status === 200) {
          void navigate({ to: "/channels/$id", params: { id: res.data.channelId } });
        }
      },
    },
  });
  const deny = proposalsApi.useDenyProposal({ mutation: { onSuccess: invalidate } });
  const feedback = useDiscoveryFeedback({ scope: "household" });

  // Filler pulls (V35). ⚠ `status: "pending"` and not a client-side filter: a decided pull is
  // KEPT on the server for the History tab, so asking for everything would put approvals an
  // operator already made back in front of them as if they still needed deciding.
  const pullsQuery = fillerApi.useListFillerPulls({ status: "pending" }, { query: { retry: false } });
  const pulls = unwrap(pullsQuery.data, (b) => b.pulls) ?? [];
  const invalidatePulls = () =>
    queryClient.invalidateQueries({ queryKey: fillerApi.getListFillerPullsQueryKey() });
  // ⚠ Approving is the COMMIT point — the only path on which a pull downloads anything.
  const approvePull = fillerApi.useApproveFillerPull({ mutation: { onSuccess: invalidatePulls } });
  const dismissPull = fillerApi.useDismissFillerPull({ mutation: { onSuccess: invalidatePulls } });

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
  const bulk = proposalsApi.useBulkApproveProposals({
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

  const rows = unwrap(proposals.data, (b) => b.proposals) ?? [];

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
      {(approve.error ?? deny.error ?? bulk.error ?? feedback.error) != null && (
        <ErrorState
          error={approve.error ?? deny.error ?? bulk.error ?? feedback.error}
          onRetry={feedback.error ? feedback.retry : undefined}
        />
      )}

      {/* Filler pulls (V35), above the title proposals. ⚠ They sit on THIS queue rather than
          getting a surface of their own because they are the same kind of decision: something
          is about to spend real resources and a human has to say yes. §10 said "the machine
          proposes, a human commits" long before there was an object to commit; this is it.
          Above rather than below because there are usually few of them and they are cheap to
          decide, so burying them under a long title list would leave them unanswered. */}
      {pulls.map((pull) => (
        <PullCard
          key={pull.id}
          pull={pull}
          deciding={approvePull.isPending || dismissPull.isPending}
          onApprove={(edits) => approvePull.mutate({ id: pull.id, data: edits })}
          onDismiss={() => dismissPull.mutate({ id: pull.id })}
        />
      ))}

      {/* Bulk approve (V27). Shown only when there is more than one approvable row — a "select
          all" above a single item is noise. */}
      {bulkable.length > 1 && (
        <div className="flex flex-wrap items-center gap-3 rounded-md border border-border bg-card px-3 py-2">
          <label htmlFor="approval-select-all" className="flex cursor-pointer items-center gap-2 text-sm">
            <Checkbox
              id="approval-select-all"
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
              {`${edited.size} edited ${edited.size === 1 ? "request is" : "requests are"} excluded. Approve those individually so the edit is kept.`}
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
              episodeSelectionPreview={p.episodeSelectionPreview}
              // What the proposal's own audience ceiling refused (§4, #259) — shown on the card
              // itself, because it changes what approving this row gets you.
              refused={p.proposal.refused ?? []}
              status={busy ? "approving" : "pending"}
              onEdit={(edit) => setEdit(p.id, edit)}
              renderFeedback={(item) => {
                const targetKey = provisionKey(item);
                if (!targetKey) return null;
                return (
                  <DiscoveryFeedbackControls
                    compact
                    name={item.name}
                    scope={{ scope: "household" }}
                    effective={feedback.feedbackFor(targetKey)}
                    disabled={feedback.isPending}
                    onSet={(action) => feedback.setFeedback(targetKey, action)}
                    onClear={() => feedback.clearFeedback(targetKey)}
                  />
                );
              }}
              // The edit rides the SAME approve call — there is no separate "save edit" step,
              // because the edit is a parameter to the one approval gate, not a mutation of the
              // proposal (§7 / D-K). An unmodified row sends `{}`, exactly as it always has:
              // the handler maps a body with no drops, adds or note to a nil edit, so approving
              // untouched carries no human modification; the server may still canonicalise a
              // missing/crafted series selector from the proposal's original Intent.
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
