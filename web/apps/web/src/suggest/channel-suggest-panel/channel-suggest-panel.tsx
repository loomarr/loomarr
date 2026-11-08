import * as discoveryApi from "@loomarr/api/endpoints/discovery";
import * as proposalsApi from "@loomarr/api/endpoints/proposals";
import type { Intent } from "@loomarr/api/models/intent";
import { toProblem } from "@loomarr/api/mutator";
import { unwrap } from "@loomarr/api/unwrap";
import { Action } from "@loomarr/design-system";
import { useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { useAuth } from "@/auth/use-auth";
import { ProposalReview } from "@/components/loomarr/ai/proposal-review";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { GenerationProgress } from "@/components/loomarr/feedback/generation-progress";
import { cn } from "@/lib/utils";
import { IntentForm } from "../intent-form";
import { useProposalOutlook } from "../live-proposal-outlook";
import { ManualChannelBuilder } from "../manual-channel-builder";
import { suggestionFailureCopy } from "../suggestion-failure-copy";
import { useAiResume } from "../use-ai-resume";
import { useElapsed } from "../use-elapsed";
import { useProposalReviewEdit } from "../use-proposal-review-edit";
import type { ChannelSuggestPanelProps } from "./channel-suggest-panel.type";

// ChannelSuggestPanel — origination, inline in the Guide header (§12). The create path IS
// describing a channel: IntentForm → the suggestion run → GenerationProgress → ProposalReview,
// and on approval it hands the new channel id back so the Guide navigates to it. It does NOT
// fork the flow or the approval gate: approve is the same admin-only useApproveProposal, and a
// member sees the review without the controls (§7/§11).
//
// This is now the app's ONLY origination surface — the standalone `/suggest` page folded away
// once this panel moved onto the channels surface. Nothing was stranded with it: the
// cross-user approval queue had already moved into `/queue`'s tabs (V27).
//
// One expanding surface over the run's three states: idle → describe form; running → live
// phases; a landed proposal → review with Approve/Deny. A successful approve or "Create
// another" resets back to the form. Which of those shows, and which Job, is the Guide's
// origination flow (its URL); this panel renders it and acts through it.
const ChannelSuggestPanel = ({ flow, onCreated, className }: ChannelSuggestPanelProps) => {
  const { isAdmin, user } = useAuth();
  const queryClient = useQueryClient();
  const { run, search } = flow;
  // "Edit first" (#1817): seeds the builder from one channel idea. Fetched here rather than in
  // the builder itself, so a plain "Add a channel"/"Build it manually instead" entry (no idea)
  // never pays for a list it doesn't need.
  const editIdeas = discoveryApi.useListChannelIdeas({ query: { enabled: search.ideaId !== undefined } });
  const editIdea = search.ideaId
    ? unwrap(editIdeas.data, (body) => body.ideas)?.find((idea) => idea.id === search.ideaId)
    : undefined;
  const [edit, setEdit, optionalSuggestions, expansionAdded] = useProposalReviewEdit(run.jobId, run.proposal);
  const { resumeWithAi } = useAiResume(run, edit, optionalSuggestions);
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
          flow.created();
          onCreated(res.data.channelId);
        }
      },
    },
  });
  const deny = proposalsApi.useDenyProposal({
    mutation: {
      onSuccess: () => {
        void queryClient.invalidateQueries({ queryKey: proposalsApi.getListProposalsQueryKey() });
        setEdit(undefined);
        // Discard is an explicit fresh start: the template and the Job leave the URL too.
        flow.startFresh();
      },
    },
  });

  const proposal = run.proposal;
  const canRevise = isAdmin && (run.actions.includes("edit") || run.isRunning);
  const outlook = useProposalOutlook({
    id: proposal?.id,
    proposal: proposal?.proposal,
    edit,
  });
  const startFresh = () => {
    setEdit(undefined);
    flow.startFresh();
  };
  const start = (intent: Intent) => {
    setEdit(undefined);
    run.start(intent);
  };
  const editFailedDescription = () => {
    setEdit(undefined);
    flow.editDescription();
  };
  const retry = () => {
    setEdit(undefined);
    run.retry();
  };
  const failureNeedsEdit = run.failure?.recoveryAction !== "retry_later";
  const failureCopy = run.failure ? suggestionFailureCopy(run.failure) : undefined;

  // "Build it manually instead" (#1817, decision G2/G2.1) on a failed or unconfigured AI run,
  // or the Guide's `?manual=1` handoff, swaps this panel for the manual builder in place rather
  // than forking a second origination surface. Leaving the builder without submitting returns
  // to the SAME error state the operator left (the run is untouched). Submitting moves the flow
  // onto the builder's Job, which leaves the builder.
  if (flow.manual) {
    return (
      <section className={cn("flex flex-col gap-4", className)}>
        <ManualChannelBuilder
          isAdmin={isAdmin}
          initialIdea={editIdea}
          onCancel={flow.leaveManual}
          onSubmitted={run.resume}
          onAiSubmitted={resumeWithAi}
          // The failed or unconfigured request that led here is history now: AI is back.
          onDescribeWithAi={flow.describeWithAi}
        />
      </section>
    );
  }

  return (
    <section className={cn("flex flex-col gap-4", className)}>
      {/* Idle — the describe form (with optional constraints). Suppressed while a run is in
          flight OR has failed: a failed run shows the failure below with its own way back,
          so falling through to a blank form here would swallow the error the user needs. */}
      {!run.isRunning && !run.failed && !proposal && (
        <IntentForm
          initialDescription={search.intent}
          initialIntent={run.intent}
          onSubmit={start}
          submitting={run.isRunning}
        />
      )}

      {run.error != null &&
        !proposal &&
        (aiUnconfigured || groundingUnconfigured ? (
          <div role="alert" className="border-border bg-muted/40 rounded-lg border p-4">
            <p className="font-medium">
              {groundingUnconfigured ? "Connect TMDB to build this channel" : "Finish AI setup"}
            </p>
            <p className="text-muted-foreground mt-1 text-sm">
              {groundingUnconfigured
                ? isAdmin
                  ? "AI is connected. Loomarr also needs TMDB to match your description to real titles. Your draft is saved."
                  : "AI is connected, but an administrator needs to connect TMDB before Loomarr can match your description to real titles. Your draft is saved."
                : isAdmin
                  ? "Connect an AI service and choose a model for channel suggestions. Your draft is saved."
                  : "An administrator needs to finish AI setup before Loomarr can build this channel. Your draft is saved."}
            </p>
            <div className="mt-2 flex flex-wrap items-center gap-2">
              {isAdmin &&
                (groundingUnconfigured ? (
                  <Action
                    accessibilityRole="link"
                    render={<Link to="/settings/connections" search={{ focus: "tmdb" }} />}
                    variant="link"
                  >
                    Connect TMDB
                  </Action>
                ) : (
                  <Action accessibilityRole="link" render={<Link to="/settings/ai" />} variant="link">
                    Set up AI
                  </Action>
                ))}
              {/* Always offered client-side on a failed generation (#1817 maintainer decision 1,
                  tracked on #1872) — no server Journey action gates it; the LLM being off is the
                  one failure this recovers from, so grounding-only failures don't offer it. */}
              {aiUnconfigured && (
                <Action onPress={flow.buildManually} variant="outline">
                  Build it manually instead
                </Action>
              )}
            </div>
          </div>
        ) : (
          <ErrorState error={run.error} />
        ))}

      {/* Running — the live generation phases. */}
      {/* Before the first frame lands the model is already loading and thinking, so
          "reasoning" is the honest default. It used to fall back to "searching", which
          announced a library search that had not started and could not be the slow part. */}
      {run.isRunning && !proposal && (
        <GenerationProgress
          phase={run.phase ?? "reasoning"}
          elapsedSeconds={run.progress ? undefined : elapsed}
          progress={run.progress}
        />
      )}

      {/* Failed — recovery copy is fixed by the authoritative Journey. Actions remain
          independently authorized by that Journey; guidance never grants a capability. */}
      {run.failed && !proposal && (
        <div
          role="alert"
          className="max-w-form border-border bg-muted/35 mx-auto flex w-full flex-col gap-3 rounded-lg border p-4"
        >
          <div>
            <h3 className="font-medium">{failureCopy?.title ?? "Something went wrong"}</h3>
            <p className="text-muted-foreground mt-1 text-sm">
              {failureCopy?.message ?? "Loomarr couldn't finish this request."}
            </p>
            {failureCopy?.guidance && (
              <p className="text-muted-foreground mt-1 text-sm">{failureCopy.guidance}</p>
            )}
          </div>
          <div className="flex flex-wrap gap-2">
            {run.actions.includes("edit") && (
              <Action onPress={editFailedDescription} variant={failureNeedsEdit ? undefined : "outline"}>
                {run.failure?.recoveryAction === "edit_reference" ? "Change link" : "Edit description"}
              </Action>
            )}
            {run.actions.includes("retry") && (
              <Action onPress={retry} variant="outline">
                Try again
              </Action>
            )}
            {run.actions.includes("check_ai") && (
              <Action accessibilityRole="link" render={<Link to="/settings/ai" />} variant="link">
                Check AI settings
              </Action>
            )}
            {/* Always offered client-side (#1817 maintainer decision 1, #1872): a generation
                failure, whatever its reason, can always fall back to building by hand. */}
            <Action onPress={flow.buildManually} variant="suggest">
              Build it manually instead
            </Action>
          </div>
        </div>
      )}

      {/* A proposal landed — review + approve/deny (approve navigates to the new channel). */}
      {proposal && (
        <div className="flex flex-col gap-4">
          <ProposalReview
            proposal={proposal.proposal}
            optionalSuggestions={optionalSuggestions}
            expansionAdded={expansionAdded}
            showWorkflowHeading={false}
            edit={edit}
            assessment={
              proposal.status === "submitted" && !outlook.isFetching && !outlook.isError
                ? outlook.data
                : undefined
            }
            assessmentPending={proposal.status === "submitted" && outlook.isFetching}
            status={edit && proposal.status === "submitted" ? "partially-edited" : proposal.status}
            selfService={isAdmin}
            busy={approve.isPending || deny.isPending}
            revising={run.isRunning}
            revisionError={
              run.error != null
                ? toProblem(run.error).title
                : run.failure != null
                  ? run.failure.message
                  : undefined
            }
            onEdit={isAdmin ? setEdit : undefined}
            onRevise={canRevise ? run.revise : undefined}
            onApprove={isAdmin ? () => approve.mutate({ id: proposal.id, data: edit ?? {} }) : undefined}
            onDeny={isAdmin ? (reason) => deny.mutate({ id: proposal.id, data: { reason } }) : undefined}
          />
          {(approve.error ?? deny.error) != null && (
            <p className="text-onair-300 text-sm">
              {toProblem(approve.error ?? deny.error).title ?? "That didn't go through. Try again."}
            </p>
          )}
          {proposal.status === "approved" && (
            <div className="flex flex-col items-start gap-2">
              <p role="status" className="text-lock text-sm">
                {user?.autoApprove
                  ? "Created automatically using your account setting."
                  : "This channel has already been created."}
              </p>
              <Action onPress={startFresh} variant="outline">
                Create another
              </Action>
            </div>
          )}
        </div>
      )}
    </section>
  );
};

export { ChannelSuggestPanel };
