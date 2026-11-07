import { readActiveSuggestionJob, readSuggestionDraft } from "../suggestion-draft";
import type { SuggestionRun } from "../use-suggestion-run/use-suggestion-run.type";

// Channel origination's state lives in the Guide's search params (§12). The panel is open while
// any of them is present, and every action is a navigation to the next set.
type OriginationSearch = {
  // Seeds the describe form: the wizard's template (§13), or the manual builder's description
  // handed back to AI.
  intent?: string;
  // The Proposal Job under review or generation. Opaque and server-authorized (My Requests).
  job?: string;
  // The empty describe form — the Requests page's "Request a channel".
  new?: "1";
  // The manual/AI-off builder (#1817, G2) — Home's "Describe your own channel".
  manual?: "1";
  // Seeds the manual builder from one channel idea — Home's "Edit first".
  ideaId?: string;
};

// The route's validateSearch, and how the panel reads the same params outside the real route.
const parseOriginationSearch = (search: Record<string, unknown>): OriginationSearch => ({
  intent: typeof search.intent === "string" ? search.intent : undefined,
  job: typeof search.job === "string" ? search.job : undefined,
  new: search.new === "1" || search.new === 1 ? "1" : undefined,
  manual: search.manual === "1" || search.manual === 1 ? "1" : undefined,
  ideaId: typeof search.ideaId === "string" ? search.ideaId : undefined,
});

const isOriginating = (search: OriginationSearch) =>
  Boolean(search.new || search.manual || search.intent || search.job);

// The tab remembers the Job it is following (and a draft that has no Job yet), so leaving the
// Guide and coming back lands on the same run. This is the only place that memory is read.
const rememberedOrigination = (): OriginationSearch | undefined => {
  const job = readActiveSuggestionJob();
  if (job) return { job };
  return readSuggestionDraft() ? { new: "1" } : undefined;
};

type OriginationStage = "describe" | "generating" | "review" | "updating" | "failed";

const originationStage = (run: SuggestionRun): OriginationStage => {
  if (run.proposal) return run.isRunning ? "updating" : "review";
  if (run.isRunning) return "generating";
  return run.failed ? "failed" : "describe";
};

// The Guide's one PageHeader names the current task, so the panel nests no heading of its own.
const originationHeading = (
  stage: OriginationStage,
  isAdmin: boolean,
): { title: string; description: string } => {
  switch (stage) {
    case "review":
      return {
        title: "Review your channel",
        description: "Choose what belongs. Nothing is created until you continue.",
      };
    case "updating":
      return {
        title: "Updating suggestions",
        description: "Your current lineup stays in place while Loomarr refreshes its suggestions.",
      };
    case "generating":
      return { title: "Finding titles", description: "Loomarr is finding titles that match your brief." };
    case "failed":
      return {
        title: "Channel suggestion",
        description: "Your brief is saved. Choose a recovery option below.",
      };
    case "describe":
      return {
        title: isAdmin ? "Add a channel" : "Request a channel",
        description:
          "Describe what you want to watch. You'll choose the final titles before anything is created.",
      };
  }
};

export type { OriginationSearch, OriginationStage };
export { isOriginating, originationHeading, originationStage, parseOriginationSearch, rememberedOrigination };
