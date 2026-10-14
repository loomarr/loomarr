import type { ApprovalEditDTO } from "@loomarr/api/models/approvalEditDTO";
import type { ProposalItem } from "@loomarr/api/models/proposalItem";
import { useEffect, useState } from "react";
import { suggestionExpansionIntent } from "../suggestion-expansion";
import type { SuggestionRun } from "../use-suggestion-run";

interface PendingResume {
  jobId: string;
  description: string;
  channelName: string;
}

// useAiResume carries a hand-built lineup into the AI path (#1817, the resume banner's
// "Try AI suggestions"). The builder has already recorded the picks as a manual proposal; this
// resumes that Job (the same `run.resume` hand-off "Continue to review" uses) and, once its
// review has landed, asks for more titles through the review's own "find more" revision.
//
// Waiting for the review is what keeps the picks: useProposalReviewEdit snapshots the landed
// manual lineup as the selection, and when the expansion's new proposal replaces it, that
// selection is carried over and the AI titles arrive as optional additions. Revising before
// the review existed would leave nothing to carry over.
//
// The name the person typed is kept the same way the picks are: the server names a revised
// proposal itself, so the typed name goes in the revision as the Intent's fixed `channelName`,
// which the server keeps over the model's. With no typed name, the model names the channel.
const useAiResume = (
  run: SuggestionRun,
  edit: ApprovalEditDTO | undefined,
  optionalSuggestions: ProposalItem[],
) => {
  const [pending, setPending] = useState<PendingResume>();
  const proposal = run.proposal?.proposal;

  useEffect(() => {
    if (!pending) return;
    // The resumed Job failed, or something else took the run over: there is no review to expand.
    if (run.failed || run.jobId !== pending.jobId) {
      setPending(undefined);
      return;
    }
    if (!proposal || run.isRunning) return;
    setPending(undefined);
    const channelName = pending.channelName.trim();
    run.revise({
      ...suggestionExpansionIntent(
        proposal,
        edit,
        [...optionalSuggestions, ...(proposal.alternates ?? [])],
        pending.description,
      ),
      ...(channelName ? { channelName } : {}),
    });
  }, [pending, run, proposal, edit, optionalSuggestions]);

  return {
    resumeWithAi: (jobId: string, description: string, channelName = "") => {
      run.resume(jobId);
      setPending({ jobId, description, channelName });
    },
    resuming: pending !== undefined,
  };
};

export { useAiResume };
