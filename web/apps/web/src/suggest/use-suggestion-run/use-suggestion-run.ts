import * as proposalsApi from "@loomarr/api/endpoints/proposals";
import type { Intent } from "@loomarr/api/models/intent";
import type { ProposalJobTrackerOptions } from "../use-proposal-job-tracker";
import { useProposalJobTracker } from "../use-proposal-job-tracker";
import type { SuggestionRun } from "./use-suggestion-run.type";

// Submission is intentionally thin: the shared tracker owns every read-side lifecycle
// rule, so origination and Refine cannot drift into separate proposal-matching machines.
const useSuggestionRun = (options: ProposalJobTrackerOptions = {}): SuggestionRun => {
  const tracker = useProposalJobTracker(options);
  const submit = proposalsApi.useSubmitProposal();

  const start = (intent: Intent) => {
    submit.mutate(
      { data: intent },
      { onSuccess: (response) => response.status === 200 && tracker.track(response.data.jobId) },
    );
  };

  return {
    ...tracker,
    isRunning: submit.isPending || tracker.isRunning,
    error: submit.error ?? tracker.error,
    start,
    retry: () => tracker.intent && start(tracker.intent),
  };
};

export { useSuggestionRun };
