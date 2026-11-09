import type { SuggestionRun } from "../use-suggestion-run/use-suggestion-run.type";

// Channel origination's state lives in the search params of its own route, `/requests/new` (§12),
// and every action is a navigation to the next set. No params is the empty describe form.
type OriginationSearch = {
  // Seeds the describe form: the wizard's template (§13), or the manual builder's description
  // handed back to AI.
  intent?: string;
  // The Proposal Job under review or generation. Opaque and server-authorized (My Requests).
  job?: string;
  // The manual/AI-off builder (#1817, G2) — Home's "Describe your own channel". A number, so the
  // router writes it as `?manual=1`, not `?manual=%221%22`.
  manual?: 1;
  // Seeds the manual builder from one channel idea — Home's "Edit first".
  ideaId?: string;
};

const flag = (value: unknown): 1 | undefined => (value === 1 || value === "1" ? 1 : undefined);

// The route's validateSearch, and how the panel reads the same params outside the real route.
const parseOriginationSearch = (search: Record<string, unknown>): OriginationSearch => ({
  intent: typeof search.intent === "string" ? search.intent : undefined,
  job: typeof search.job === "string" ? search.job : undefined,
  manual: flag(search.manual),
  ideaId: typeof search.ideaId === "string" ? search.ideaId : undefined,
});

// The flow used to live on the Guide route (`/guide?new=1`, `?job=`, `?manual=1`, `?intent=`). Where
// an old URL of that kind should go now: the request route's params, or nothing for an ordinary
// Guide visit. `?new=1` was the empty describe form, which is now just the route itself.
const legacyGuideOrigination = (search: Record<string, unknown>): OriginationSearch | undefined => {
  const next = parseOriginationSearch(search);
  const arrived = Boolean(flag(search.new) || next.manual || next.intent || next.job || next.ideaId);
  if (!arrived) return undefined;
  return Object.fromEntries(Object.entries(next).filter(([, value]) => value !== undefined));
};

type OriginationStage = "describe" | "generating" | "review" | "updating" | "failed";

const originationStage = (run: SuggestionRun): OriginationStage => {
  if (run.proposal) return run.isRunning ? "updating" : "review";
  if (run.isRunning) return "generating";
  return run.failed ? "failed" : "describe";
};

// The page's one PageHeader names the current task, so the panel nests no heading of its own.
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
export { legacyGuideOrigination, originationHeading, originationStage, parseOriginationSearch };
