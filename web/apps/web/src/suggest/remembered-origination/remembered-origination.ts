import type { OriginationSearch } from "../origination-flow";
import { readActiveSuggestionJob, readSuggestionDraft } from "../suggestion-draft";

// The tab remembers the Job it is following (and a draft that has no Job yet), so leaving the
// Guide and coming back lands on the same run. This is the only place that memory is read.
//
// Its own module because reading the draft validates it with a zod schema: the Guide route's
// guard imports this on entry, so the schemas stay out of the first download.
const rememberedOrigination = (): OriginationSearch | undefined => {
  const job = readActiveSuggestionJob();
  if (job) return { job };
  return readSuggestionDraft() ? { new: "1" } : undefined;
};

export { rememberedOrigination };
