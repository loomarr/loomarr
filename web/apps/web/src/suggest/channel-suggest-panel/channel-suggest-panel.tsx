import * as proposalsApi from "@loomarr/api/endpoints/proposals";
import type { ApprovalEditDTO } from "@loomarr/api/models/approvalEditDTO";
import type { Intent } from "@loomarr/api/models/intent";
import { toProblem } from "@loomarr/api/mutator";
import { useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { useState } from "react";
import { useAuth } from "@/auth/use-auth";
import { ProposalReview } from "@/components/loomarr/ai/proposal-review";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { GenerationProgress } from "@/components/loomarr/feedback/generation-progress";
import { Button, buttonVariants } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { IntentForm } from "../intent-form";
import { LiveProposalOutlook } from "../live-proposal-outlook";
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
const ChannelSuggestPanel = ({
  onCreated,
  initialIntent,
  initialJobId,
  onStartFresh,
  className,
}: ChannelSuggestPanelProps) => {
  const { isAdmin, user } = useAuth();
  const queryClient = useQueryClient();
  const [startedFresh, setStartedFresh] = useState(false);
  const [edit, setEdit] = useState<ApprovalEditDTO | undefined>();
  const run = useSuggestionRun(initialJobId);
  const elapsed = useElapsed(run.isRunning);
  const runProblem = run.error == null ? undefined : toProblem(run.error);
  const aiUnconfigured = runProblem?.type === "feature_not_configured";
  const groundingUnconfigured = runProblem?.type === "grounding_not_configured";

  const approve = proposalsApi.useApproveProposal({
    mutation: {
      onSuccess: (res) => {
        void queryClient.invalidateQueries({ queryKey: proposalsApi.getListProposalsQueryKey() });
        // Approval atomically created (or patched) the local channel and returned its required
        // id — navigate there so the operator lands on the channel it just committed.
        if (res.status === 200) {
          setEdit(undefined);
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
  const startFresh = () => {
    setEdit(undefined);
    run.reset();
    setStartedFresh(true);
    onStartFresh?.();
  };
  const start = (intent: Intent) => {
    setEdit(undefined);
    run.start(intent);
  };
  const editDescription = () => {
    setEdit(undefined);
    run.reset(true);
  };
  const retry = () => {
    setEdit(undefined);
    run.retry();
  };
  const failureNeedsEdit = run.failure?.recoveryAction !== "retry_later";
  const failureTitle =
    run.failure?.recoveryAction === "simplify_request"
      ? "Try a more specific description"
      : failureNeedsEdit
        ? "Adjust your description"
        : "We couldn't finish this channel";
  const failureMessage =
    run.failure?.reason === "discovery_budget_exhausted"
      ? "Loomarr found too many possible directions. Add a decade, genre, network, or a few example titles."
      : (run.failure?.message ?? "Something interrupted this channel. Your description is still here.");

  return (
    <section className={cn("flex flex-col gap-4", className)}>
      {/* Idle — the describe form (with optional constraints). Suppressed while a run is in
          flight OR has failed: a failed run shows the failure below with its own way back,
          so falling through to a blank form here would swallow the error the user needs. */}
      {!run.isRunning && !run.failed && !proposal && (
        <IntentForm
          initialDescription={startedFresh ? undefined : initialIntent}
          initialIntent={run.intent}
          onSubmit={start}
          submitting={run.isRunning}
        />
      )}

      {run.error != null &&
        (aiUnconfigured || groundingUnconfigured ? (
          <div role="alert" className="rounded-lg border border-border bg-muted/40 p-4">
            <p className="font-medium">
              {groundingUnconfigured ? "Connect TMDB to build this channel" : "Finish AI setup"}
            </p>
            <p className="mt-1 text-muted-foreground text-sm">
              {groundingUnconfigured
                ? isAdmin
                  ? "AI is connected. Loomarr also needs TMDB to match your description to real titles. Your draft is saved."
                  : "AI is connected, but an administrator needs to connect TMDB before Loomarr can match your description to real titles. Your draft is saved."
                : isAdmin
                  ? "Connect a provider and choose a lineup model. Your draft is saved."
                  : "An administrator needs to finish AI setup before Loomarr can build this channel. Your draft is saved."}
            </p>
            {isAdmin &&
              (groundingUnconfigured ? (
                <Link
                  to="/settings/connections"
                  search={{ focus: "tmdb" }}
                  className={buttonVariants({ variant: "link", size: "sm" })}
                >
                  Connect TMDB
                </Link>
              ) : (
                <Link to="/settings/ai" className={buttonVariants({ variant: "link", size: "sm" })}>
                  Set up AI
                </Link>
              ))}
          </div>
        ) : (
          <ErrorState error={run.error} />
        ))}

      {/* Running — the live generation phases. */}
      {/* Before the first frame lands the model is already loading and thinking, so
          "reasoning" is the honest default. It used to fall back to "searching", which
          announced a library search that had not started and could not be the slow part. */}
      {run.isRunning && <GenerationProgress phase={run.phase ?? "reasoning"} elapsedSeconds={elapsed} />}

      {/* Failed — recovery copy is fixed by the authoritative Journey. Actions remain
          independently authorized by that Journey; guidance never grants a capability. */}
      {run.failed && (
        <div
          role="alert"
          className="mx-auto flex w-full max-w-2xl flex-col gap-3 rounded-lg border border-border bg-muted/35 p-4"
        >
          <div>
            <h3 className="font-medium">{failureTitle}</h3>
            <p className="mt-1 text-muted-foreground text-sm">{failureMessage}</p>
          </div>
          <div className="flex flex-wrap gap-2">
            {run.actions.includes("edit") && (
              <Button variant={failureNeedsEdit ? "default" : "outline"} size="sm" onClick={editDescription}>
                {run.failure?.recoveryAction === "edit_reference" ? "Change reference" : "Edit description"}
              </Button>
            )}
            {run.actions.includes("retry") && (
              <Button variant="outline" size="sm" onClick={retry}>
                Try again
              </Button>
            )}
            {run.actions.includes("check_ai") && (
              <Link to="/settings/ai" className={buttonVariants({ variant: "link", size: "sm" })}>
                Check AI settings
              </Link>
            )}
          </div>
        </div>
      )}

      {/* A proposal landed — review + approve/deny (approve navigates to the new channel). */}
      {proposal && (
        <div className="flex flex-col gap-4">
          <ProposalReview
            proposal={proposal.proposal}
            edit={edit}
            outlook={
              proposal.status === "submitted" ? (
                <LiveProposalOutlook
                  id={proposal.id}
                  proposal={proposal.proposal}
                  edit={edit}
                  onAddVariety={approve.isPending || deny.isPending ? undefined : editDescription}
                />
              ) : undefined
            }
            status={edit && proposal.status === "submitted" ? "partially-edited" : proposal.status}
            selfService={isAdmin}
            busy={approve.isPending || deny.isPending}
            onEdit={isAdmin ? setEdit : undefined}
            onEditRequest={editDescription}
            onApprove={isAdmin ? () => approve.mutate({ id: proposal.id, data: edit ?? {} }) : undefined}
            onDeny={isAdmin ? (reason) => deny.mutate({ id: proposal.id, data: { reason } }) : undefined}
          />
          {(approve.error ?? deny.error) != null && (
            <p className="text-onair-300 text-sm">
              {toProblem(approve.error ?? deny.error).title ?? "That didn't go through. Try again."}
            </p>
          )}
          {proposal.status === "approved" ? (
            <div className="flex flex-col items-start gap-2">
              <p role="status" className="text-lock text-sm">
                {user?.autoApprove
                  ? "Automatically approved using your account setting. The channel has already been created."
                  : "This proposal is already approved. The channel has already been created."}
              </p>
              <Button variant="outline" size="sm" onClick={startFresh}>
                Create another
              </Button>
            </div>
          ) : (
            <Button variant="outline" size="sm" className="w-fit" onClick={startFresh}>
              Start over
            </Button>
          )}
        </div>
      )}
    </section>
  );
};

export { ChannelSuggestPanel };
