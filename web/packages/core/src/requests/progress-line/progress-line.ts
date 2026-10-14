import type { ProposalJourneyProgressDTO } from "@loomarr/api/models/proposalJourneyProgressDTO";

// The stage line for a run's real backend stage. Terms are the model's own catalog_search
// arguments, so the words are what was actually searched — never a paraphrase. One place, so
// the Guide, the Requests card and the request detail can never word the same moment differently.
const progressLine = (progress: ProposalJourneyProgressDTO): string => {
  switch (progress.stage) {
    case "searching":
      return progress.terms.length > 0
        ? `Searching your library for ${progress.terms.join(", ")}…`
        : "Searching your library…";
    case "choosing":
      return "Choosing titles…";
    case "building":
      return "Building your lineup…";
    default:
      return "Reading your request…";
  }
};

// "4 of about 8 picked" — the denominator is the most titles the model is asked for, not a guess.
const pickCount = (progress: ProposalJourneyProgressDTO): string | undefined =>
  progress.picks.length > 0 ? `${progress.picks.length} of about ${progress.target} picked` : undefined;

export { pickCount, progressLine };
