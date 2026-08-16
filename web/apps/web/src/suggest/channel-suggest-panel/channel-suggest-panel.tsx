import * as proposalsApi from "@loomarr/api/endpoints/proposals";
import { toProblem } from "@loomarr/api/mutator";
import { useQueryClient } from "@tanstack/react-query";
import { useAuth } from "@/auth/use-auth";
import { ProposalReview } from "@/components/loomarr/ai/proposal-review";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { GenerationProgress } from "@/components/loomarr/feedback/generation-progress";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { IntentForm } from "../intent-form";
import { useElapsed } from "../use-elapsed";
import { useSuggestionRun } from "../use-suggestion-run";
import type { ChannelSuggestPanelProps } from "./channel-suggest-panel.type";

// ChannelSuggestPanel — origination, inline in the Guide header (§12). The create path IS
// describing a channel: IntentForm → useSuggestionRun → GenerationProgress → ProposalReview,
// and on approval it hands the new channel id back so the Guide navigates to it. It does NOT
// fork the flow or the approval gate: approve is the same admin-only useApproveProposal, and a
// member sees the review without the controls (§7/§11).
//
// This is now the app's ONLY origination surface — the standalone `/suggest` page folded away
// once this panel moved onto the channels surface. Nothing was stranded with it: the
// cross-user approval queue had already moved into `/queue`'s tabs (V27).
//
// One expanding surface over useSuggestionRun's three states: idle → describe form; running →
// live phases; a landed proposal → review with Approve/Deny. A successful approve or "Start
// another" resets back to the form.
const ChannelSuggestPanel = ({ onCreated, initialIntent, className }: ChannelSuggestPanelProps) => {
  const { isAdmin } = useAuth();
  const queryClient = useQueryClient();
  const run = useSuggestionRun();
  const elapsed = useElapsed(run.isRunning);

  const approve = proposalsApi.useApproveProposal({
    mutation: {
      onSuccess: (res) => {
        void queryClient.invalidateQueries({ queryKey: proposalsApi.getListProposalsQueryKey() });
        // Approval atomically created (or patched) the local channel and returned its required
        // id — navigate there so the operator lands on the channel it just committed.
        if (res.status === 200) {
          run.reset();
          onCreated(res.data.channelId);
        }
      },
    },
  });
  const deny = proposalsApi.useDenyProposal({
    mutation: {
      onSuccess: () => {
        void queryClient.invalidateQueries({ queryKey: proposalsApi.getListProposalsQueryKey() });
        run.reset();
      },
    },
  });

  const proposal = run.proposal;

  return (
    <section className={cn("flex flex-col gap-4 rounded-lg border border-border p-4", className)}>
      <div>
        <h2 className="font-semibold text-lg">Add a channel</h2>
        <p className="text-muted-foreground text-sm">
          Describe the channel you want. Loomarr grounds every pick against your library and TMDB, then you
          review and approve before anything is built.
        </p>
      </div>

      {/* Idle — the describe form (with optional constraints). */}
      {!run.isRunning && !proposal && (
        <IntentForm initialIntent={initialIntent} onSubmit={run.start} submitting={run.isRunning} />
      )}

      {run.error != null && <ErrorState error={run.error} />}

      {/* Running — the live generation phases. */}
      {/* Before the first frame lands the model is already loading and thinking, so
          "reasoning" is the honest default. It used to fall back to "searching", which
          announced a library search that had not started and could not be the slow part. */}
      {run.isRunning && (
        <GenerationProgress phase={run.phase ?? "reasoning"} round={run.round} elapsedSeconds={elapsed} />
      )}

      {/* A proposal landed — review + approve/deny (approve navigates to the new channel). */}
      {proposal && (
        <div className="flex flex-col gap-4">
          <ProposalReview
            proposal={proposal.proposal}
            status={proposal.status}
            busy={approve.isPending || deny.isPending}
            onApprove={isAdmin ? () => approve.mutate({ id: proposal.id, data: {} }) : undefined}
            onDeny={isAdmin ? (reason) => deny.mutate({ id: proposal.id, data: { reason } }) : undefined}
          />
          {(approve.error ?? deny.error) != null && (
            <p className="text-onair-300 text-sm">
              {toProblem(approve.error ?? deny.error).title ?? "That didn't go through. Try again."}
            </p>
          )}
          <Button variant="outline" size="sm" className="w-fit" onClick={run.reset}>
            Start over
          </Button>
        </div>
      )}
    </section>
  );
};

export { ChannelSuggestPanel };
