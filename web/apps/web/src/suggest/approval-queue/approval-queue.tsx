import * as channelsApi from "@loomarr/api/endpoints/channels";
import * as fillerApi from "@loomarr/api/endpoints/filler";
import * as proposalsApi from "@loomarr/api/endpoints/proposals";
import type { ApprovalEditDTO } from "@loomarr/api/models/approvalEditDTO";
import type { ChannelDTO } from "@loomarr/api/models/channelDTO";
import type { ProposalDTO } from "@loomarr/api/models/proposalDTO";
import type { PullDTO } from "@loomarr/api/models/pullDTO";
import { isOk, unwrap } from "@loomarr/api/unwrap";
import { provisionKey } from "@loomarr/core/provision";
import { approvalChannelEffect } from "@loomarr/core/requests";
import { useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { useState } from "react";
import { ApprovalGroup } from "@/components/loomarr/ai/approval-group";
import { ApprovalQueueItem } from "@/components/loomarr/ai/approval-queue-item";
import {
  type BulkApproveConsequences,
  BulkApproveDialog,
  type BulkApproveOutcome,
  type ChannelRequestConsequence,
  type FillerDownloadConsequence,
} from "@/components/loomarr/ai/bulk-approve-dialog";
import { EmptyState } from "@/components/loomarr/feedback/empty-state";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { PullCard } from "@/components/loomarr/filler/pull-card";
import { DiscoveryFeedbackControls, useDiscoveryFeedback } from "@/discovery-feedback";
import { LiveProposalOutlook } from "../live-proposal-outlook";

// The admin approval queue (§7, §11) — the human gate every acquisition passes through.
// It lists everything still `submitted`, which is exactly what the status filter means.
// Approving is the ONLY path from a proposal to /v1/titles, so this list is the audit
// surface for "what is about to spend real resources".

// Inside a group the rows are flat, divided by the group's own rules (mock #1876), so the row
// components drop their card chrome rather than nesting a card in a card.
const FLAT_ROW = "border-0 bg-transparent p-0";

const requestTitle = (p: ProposalDTO) => p.proposal.intent?.description ?? "Suggested lineup";

// What approving one request will do, from the server's own data (#1817): the counts from the
// proposal, sizes from its items (#1885), the channel from the job binding approval uses.
const channelConsequence = (p: ProposalDTO, channels: readonly ChannelDTO[]): ChannelRequestConsequence => {
  const lineup = p.proposal.lineup ?? [];
  const acquisitions = p.proposal.acquisitions ?? [];
  const ceiling = p.proposal.policy?.audience?.ceiling;
  return {
    id: p.id,
    title: requestTitle(p),
    titleCount: lineup.length + acquisitions.length,
    toDownload: acquisitions.length,
    ...(ceiling ? { ceiling } : {}),
    sizes: [...lineup, ...acquisitions],
    channel: approvalChannelEffect(p.jobId, p.proposal, channels),
  };
};

const fillerConsequence = (pull: PullDTO): FillerDownloadConsequence => ({
  id: pull.id,
  title: pull.title,
  estimateClips: pull.estimateClips,
  sources: new Set((pull.plan ?? []).filter((row) => !row.dropped).map((row) => row.sourceId)).size,
});

// A retry answers for only the ids it re-sent; every other row keeps the answer it had.
const mergeOutcomes = (
  prev: readonly BulkApproveOutcome[] | undefined,
  next: readonly BulkApproveOutcome[],
): BulkApproveOutcome[] => (prev ? prev.map((r) => next.find((n) => n.id === r.id) ?? r) : [...next]);

const without = (set: ReadonlySet<string>, id: string) => {
  const next = new Set(set);
  next.delete(id);
  return next;
};

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

  // ⚠ Approval binds a channel by JOB: a revised request replaces the channel its job already
  // has rather than making a second one. The consequence summary reads this list to say which,
  // so a group's Approve waits for it rather than guessing "new channel" (#1817).
  const channelsQuery = channelsApi.useListChannels({ query: { staleTime: 30_000, retry: false } });
  const channels = unwrap(channelsQuery.data, (b) => b.channels) ?? [];

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
  // Pulls whose card holds a left-out row or a note. The card owns the edit; the queue only
  // needs to know it exists, to keep that pull out of bulk.
  const [editedPulls, setEditedPulls] = useState<ReadonlySet<string>>(new Set());

  // Bulk selection (V27), one set per group (R1): the groups never share a bulk action.
  const [selectedProposals, setSelectedProposals] = useState<ReadonlySet<string>>(new Set());
  const [selectedPulls, setSelectedPulls] = useState<ReadonlySet<string>>(new Set());

  // An edit takes its row out of bulk, so it leaves the selection too — otherwise undoing the
  // edit would bring back a selection the operator no longer remembers making.
  const setEdit = (id: string, edit: ApprovalEditDTO | undefined) => {
    setEdits((prev) => ({ ...prev, [id]: edit }));
    if (edit) setSelectedProposals((prev) => without(prev, id));
  };
  const setPullEdited = (id: string, edited: boolean) => {
    setEditedPulls((prev) => (edited ? new Set(prev).add(id) : without(prev, id)));
    if (edited) setSelectedPulls((prev) => without(prev, id));
  };

  // The consequence summary and, once the call answers, its result. The items are captured when
  // the dialog opens: approved rows leave the list as it refetches, and the result still has to
  // name them.
  const [review, setReview] = useState<{
    consequences: BulkApproveConsequences;
    results?: BulkApproveOutcome[];
  } | null>(null);

  // Bulk approve goes to its own endpoint per group, which loops the SINGLE approve handler
  // server-side — so the gate stays one implementation. A retry is the same call with only the
  // failed ids.
  const bulk = proposalsApi.useBulkApproveProposals({ mutation: { onSuccess: invalidate } });
  const bulkPulls = fillerApi.useBulkApproveFillerPulls({ mutation: { onSuccess: invalidatePulls } });
  const recordOutcomes = (outcomes: readonly BulkApproveOutcome[] | null | undefined) =>
    setReview((prev) => prev && { ...prev, results: mergeOutcomes(prev.results, outcomes ?? []) });
  const submitBulk = (ids: string[]) => {
    if (review?.consequences.group === "channels") {
      bulk.mutate({ data: { ids } }, { onSuccess: (res) => isOk(res) && recordOutcomes(res.data.results) });
    } else {
      bulkPulls.mutate(
        { data: { ids } },
        { onSuccess: (res) => isOk(res) && recordOutcomes(res.data.results) },
      );
    }
  };
  // Cancel keeps the selection exactly as it was: nothing was approved.
  const closeReview = () => {
    setReview(null);
    bulk.reset();
    bulkPulls.reset();
  };
  // Done clears that group's selection: the ids that went through are no longer pending, and a
  // stale selection would offer to approve them again.
  const finishReview = () => {
    if (review?.consequences.group === "channels") setSelectedProposals(new Set());
    else setSelectedPulls(new Set());
    closeReview();
  };

  if (proposals.error) {
    return <ErrorState error={proposals.error} onRetry={() => proposals.refetch()} />;
  }

  const rows = unwrap(proposals.data, (b) => b.proposals) ?? [];

  // ⚠ Pulls count: with no proposals but a pending pull, "Queue's clear" would hide the one thing
  // waiting on a human (#1404). A pulls FETCH ERROR also keeps us out of the empty state — "clear"
  // must not mean "couldn't look".
  if (rows.length === 0 && pulls.length === 0 && !pullsQuery.error) {
    return (
      <EmptyState
        title="Queue's clear"
        description="Nothing is waiting for approval. Suggested lineups land here before anything is acquired."
      />
    );
  }

  const busy =
    approve.isPending || deny.isPending || bulk.isPending || bulkPulls.isPending || review !== null;

  // Which rows a decision is in flight FOR. `busy` disables the shared controls, but only the row
  // actually being decided shows "approving" — flagging every row made a two-row queue read as if
  // both were being approved (#1404).
  const deciding = new Set<string>([
    ...(approve.isPending && approve.variables ? [approve.variables.id] : []),
    ...(deny.isPending && deny.variables ? [deny.variables.id] : []),
    // Object.values: the generated body type maps `string[]` to an index-signature object.
    ...(bulk.isPending ? Object.values(bulk.variables?.data.ids ?? []) : []),
  ]);
  const decidingPulls = new Set<string>(
    bulkPulls.isPending ? Object.values(bulkPulls.variables?.data.ids ?? []) : [],
  );

  // ⚠ A row with a PENDING EDIT is excluded from bulk. The bulk endpoint takes no edit field,
  // deliberately — "drop these two titles" means nothing applied across a batch — so bulk-
  // approving an edited row would silently discard the admin's edit and acquire the titles they
  // just removed. Those rows keep their own Approve button, which does carry the edit. A pull's
  // left-out rows and note are the same kind of edit, under the same rule.
  const isEdited = (id: string) => edits[id] !== undefined;

  const openReview = (consequences: BulkApproveConsequences) => {
    bulk.reset();
    bulkPulls.reset();
    setReview({ consequences });
  };

  // A single proposal's approve/deny error (#1430): rendered on THAT row by ApprovalQueueItem,
  // not in a page-level banner here — a reviewer scanning other rows should not have to go
  // looking for why one of them didn't go through.
  const rowError = (id: string): unknown =>
    (approve.isError && approve.variables?.id === id ? approve.error : undefined) ??
    (deny.isError && deny.variables?.id === id ? deny.error : undefined);

  return (
    <div className="flex flex-col gap-2.5">
      {/* Feedback and lookup errors are not about one row, so they stay here. A bulk call's
          failure is shown inside its own dialog, which is where the operator is looking. */}
      {feedback.error != null && <ErrorState error={feedback.error} onRetry={feedback.retry} />}
      {channelsQuery.error != null && (
        <ErrorState error={channelsQuery.error} onRetry={() => channelsQuery.refetch()} />
      )}
      {pullsQuery.error != null && (
        <ErrorState error={pullsQuery.error} onRetry={() => pullsQuery.refetch()} />
      )}

      {/* Two groups, each with its own bulk scope (R1, mock #1876): channel requests and filler
          downloads are different decisions about different resources, and one Approve over both
          could not say what it was about to do. */}
      {rows.length > 0 && (
        <ApprovalGroup
          title="Channel requests"
          selected={selectedProposals}
          onSelectedChange={setSelectedProposals}
          // Waits for the channel list: without it the summary cannot tell "new channel" from
          // "updates the channel this request already has".
          disabled={busy || !channelsQuery.isSuccess}
          onApprove={() =>
            openReview({
              group: "channels",
              items: rows
                .filter((p) => selectedProposals.has(p.id) && !isEdited(p.id))
                .map((p) => channelConsequence(p, channels)),
            })
          }
          rows={rows.map((p) => ({
            id: p.id,
            label: requestTitle(p),
            excluded: isEdited(p.id),
            content: (
              <ApprovalQueueItem
                className={FLAT_ROW}
                title={requestTitle(p)}
                requestedBy={p.createdByName}
                summary={p.proposal.rationale}
                outlook={() => <LiveProposalOutlook id={p.id} proposal={p.proposal} edit={edits[p.id]} />}
                acquisitions={p.proposal.acquisitions?.length ?? 0}
                lineup={p.proposal.lineup ?? []}
                acquisitionItems={p.proposal.acquisitions ?? []}
                alternates={p.proposal.alternates ?? []}
                episodeSelectionPreview={p.episodeSelectionPreview}
                // What the proposal's own audience ceiling refused (§4, #259) — shown on the card
                // itself, because it changes what approving this row gets you.
                refused={p.proposal.refused ?? []}
                status={deciding.has(p.id) ? "approving" : "pending"}
                error={rowError(p.id)}
                edit={edits[p.id]}
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
                onApprove={() => {
                  const pending = edits[p.id];
                  const note = pending?.note?.trim();
                  const data = pending
                    ? {
                        ...(pending.drop?.length ? { drop: pending.drop } : {}),
                        ...(pending.add?.length ? { add: pending.add } : {}),
                        ...(note ? { note } : {}),
                      }
                    : {};
                  approve.mutate({ id: p.id, data });
                }}
                onDeny={(reason) => deny.mutate({ id: p.id, data: { reason } })}
              />
            ),
          }))}
        />
      )}

      {/* Filler pulls (V35). ⚠ They sit on THIS queue rather than getting a surface of their own
          because they are the same kind of decision: something is about to spend real resources
          and a human has to say yes. §10 said "the machine proposes, a human commits" long before
          there was an object to commit; this is it. */}
      {pulls.length > 0 && (
        <ApprovalGroup
          title="Filler downloads"
          // Only when it is true of every pull: a legacy pull a person proposed names them on its
          // own card, and the group must not contradict it.
          {...(pulls.every((pull) => !pull.proposedByName) ? { requestedBy: "Requested by Loomarr" } : {})}
          selected={selectedPulls}
          onSelectedChange={setSelectedPulls}
          disabled={busy}
          onApprove={() =>
            openReview({
              group: "filler",
              items: pulls
                .filter((pull) => selectedPulls.has(pull.id) && !editedPulls.has(pull.id))
                .map(fillerConsequence),
            })
          }
          rows={pulls.map((pull) => ({
            id: pull.id,
            label: pull.title,
            excluded: editedPulls.has(pull.id),
            content: (
              <PullCard
                className={FLAT_ROW}
                pull={pull}
                deciding={
                  decidingPulls.has(pull.id) ||
                  (approvePull.isPending && approvePull.variables?.id === pull.id) ||
                  (dismissPull.isPending && dismissPull.variables?.id === pull.id)
                }
                error={
                  (approvePull.isError && approvePull.variables?.id === pull.id
                    ? approvePull.error
                    : undefined) ??
                  (dismissPull.isError && dismissPull.variables?.id === pull.id
                    ? dismissPull.error
                    : undefined)
                }
                onEditedChange={(edited) => setPullEdited(pull.id, edited)}
                onApprove={(pullEdits) => approvePull.mutate({ id: pull.id, data: pullEdits })}
                onDismiss={() => dismissPull.mutate({ id: pull.id })}
              />
            ),
          }))}
        />
      )}

      {review && (
        <BulkApproveDialog
          open
          consequences={review.consequences}
          {...(review.results ? { results: review.results } : {})}
          pending={bulk.isPending || bulkPulls.isPending}
          error={bulk.error ?? bulkPulls.error}
          onCancel={closeReview}
          onConfirm={() => submitBulk(review.consequences.items.map((i) => i.id))}
          onRetry={submitBulk}
          onDone={finishReview}
        />
      )}
    </div>
  );
};

export { ApprovalQueue };
