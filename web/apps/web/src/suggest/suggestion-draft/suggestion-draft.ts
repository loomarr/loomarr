import type { Intent } from "@loomarr/api/models/intent";
import { intentSchema } from "@loomarr/core/schemas";

const SUGGESTION_DRAFT_KEY = "loomarr.pendingProposalIntent";

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

export { clearSuggestionDraft, readSuggestionDraft, SUGGESTION_DRAFT_KEY, writeSuggestionDraft };
