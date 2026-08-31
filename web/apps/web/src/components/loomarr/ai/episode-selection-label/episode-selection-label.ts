import type { EpisodeSelection } from "@loomarr/api/models/episodeSelection";
import type { ProposalItem } from "@loomarr/api/models/proposalItem";

// episodeSelectionLabel is the one reviewer-facing interpretation of the
// server-owned selector. Legacy/omitted series are the complete deck; movies
// have no episode selector at all.
const episodeSelectionLabel = (item: ProposalItem, preview?: EpisodeSelection): string | null => {
  const selection = item.episodeSelection ?? (item.mediaType === "series" ? preview : undefined);
  switch (selection?.mode) {
    case "complete":
      return "All episodes";
    case "highlights":
      return "Curated highlights";
    case "holiday": {
      const holidays = selection.holidays ?? [];
      return holidays.length > 0 ? `${holidays.join(", ")} episodes` : "Holiday episodes";
    }
    default:
      return item.mediaType === "series" ? "All episodes" : null;
  }
};

export { episodeSelectionLabel };
