import type { Intent } from "@loomarr/api/models/intent";
import { intentSchema } from "@loomarr/core/schemas";

const SUGGESTION_DRAFT_KEY = "loomarr.pendingProposalIntent";
const ACTIVE_SUGGESTION_JOB_KEY = "loomarr.activeProposalJob";

const readSuggestionDraft = (): Intent | undefined => {
  if (typeof window === "undefined") return undefined;
  const raw = window.sessionStorage.getItem(SUGGESTION_DRAFT_KEY);
  if (!raw) return undefined;
  try {
    const parsed = intentSchema.safeParse(JSON.parse(raw));
    if (parsed.success) return parsed.data as Intent;
  } catch {
    // A stale/corrupt browser value is not a user draft. Remove it below so it cannot
    // keep reopening Add channel forever.
  }
  window.sessionStorage.removeItem(SUGGESTION_DRAFT_KEY);
  return undefined;
};

const writeSuggestionDraft = (intent: Intent) => {
  if (typeof window !== "undefined") {
    window.sessionStorage.setItem(SUGGESTION_DRAFT_KEY, JSON.stringify(intent));
  }
};

const clearSuggestionDraft = () => {
  if (typeof window !== "undefined") window.sessionStorage.removeItem(SUGGESTION_DRAFT_KEY);
};

const readActiveSuggestionJob = (): string | undefined => {
  if (typeof window === "undefined") return undefined;
  return window.sessionStorage.getItem(ACTIVE_SUGGESTION_JOB_KEY) ?? undefined;
};

const writeActiveSuggestionJob = (jobId: string | undefined) => {
  if (typeof window === "undefined") return;
  if (jobId) window.sessionStorage.setItem(ACTIVE_SUGGESTION_JOB_KEY, jobId);
  else window.sessionStorage.removeItem(ACTIVE_SUGGESTION_JOB_KEY);
};

export {
  ACTIVE_SUGGESTION_JOB_KEY,
  clearSuggestionDraft,
  readActiveSuggestionJob,
  readSuggestionDraft,
  SUGGESTION_DRAFT_KEY,
  writeActiveSuggestionJob,
  writeSuggestionDraft,
};
