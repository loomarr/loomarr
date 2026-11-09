import type { OriginationSearch } from "../origination-flow";
import { readActiveSuggestionJob } from "../suggestion-draft";

// The tab remembers the Job it is following, so leaving the request page and coming back lands on
// the same run. This is the only place that memory is read. (A draft with no Job yet needs no
// remembering: the describe form reads it itself.)
//
// Its own module so the request route's guard can import it on entry, keeping the draft's zod
// schemas out of the first download.
const rememberedOrigination = (): OriginationSearch | undefined => {
  const job = readActiveSuggestionJob();
  return job ? { job } : undefined;
};

export { rememberedOrigination };
