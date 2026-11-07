import { useNavigate } from "@tanstack/react-router";
import { useEffect } from "react";
import {
  isOriginating,
  type OriginationSearch,
  type OriginationStage,
  originationStage,
  rememberedOrigination,
} from "../origination-flow";
import { clearSuggestionDraft, writeActiveSuggestionJob } from "../suggestion-draft";
import { useSuggestionRun } from "../use-suggestion-run";
import type { SuggestionRun } from "../use-suggestion-run/use-suggestion-run.type";

type OriginationFlow = {
  search: OriginationSearch;
  run: SuggestionRun;
  open: boolean;
  // The manual/AI-off builder is showing in place of the describe → review flow.
  manual: boolean;
  stage: OriginationStage;
  openPanel: () => void;
  close: () => void;
  startFresh: () => void;
  // Back to the describe form with the failed request's complete intent.
  editDescription: () => void;
  buildManually: () => void;
  leaveManual: () => void;
  // The manual builder hands its description to AI.
  describeWithAi: (description: string) => void;
  // An approval created the channel; the caller navigates to it.
  created: () => void;
};

// Channel origination on the Guide (§12) as one flow over the route's search params. Every
// action is a navigation to the next params, `replace`d so the panel's steps stay out of history.
// The run follows the params' Job; a submit or resume moves the params onto the new Job.
const useOriginationFlow = (search: OriginationSearch): OriginationFlow => {
  const navigate = useNavigate();
  const go = (next: OriginationSearch) => navigate({ to: "/guide", search: next, replace: true });
  const run = useSuggestionRun({ jobId: search.job, onJob: (job) => go({ job }) });

  // The tab remembers the Job it is following, so leaving the Guide and coming back, or closing
  // the panel and reopening it, lands on the same run (rememberedOrigination).
  useEffect(() => {
    if (search.job) writeActiveSuggestionJob(search.job);
  }, [search.job]);

  // Leaves the current Job for good: neither reopening nor a reload brings it back.
  const forget = (preserveIntent = false) => {
    run.reset(preserveIntent);
    writeActiveSuggestionJob(undefined);
  };

  return {
    search,
    run,
    open: isOriginating(search),
    manual: search.manual === "1",
    stage: originationStage(run),
    openPanel: () => void go(rememberedOrigination() ?? { new: "1" }),
    // Close means "come back later": the Job stays remembered, an unsubmitted draft does not.
    close: () => {
      clearSuggestionDraft();
      void go({});
    },
    startFresh: () => {
      forget();
      void go({ new: "1" });
    },
    editDescription: () => {
      forget(true);
      void go({ new: "1" });
    },
    buildManually: () => void go({ ...search, manual: "1" }),
    // Back to where the builder was entered from: the same failed Job, or the describe form.
    leaveManual: () => void go(search.job ? { job: search.job } : { new: "1" }),
    describeWithAi: (description) => {
      forget();
      void go({ new: "1", intent: description || undefined });
    },
    created: () => forget(),
  };
};

export type { OriginationFlow };
export { useOriginationFlow };
