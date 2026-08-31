import type { EpisodeSelection } from "@loomarr/api/models/episodeSelection";
import type { ProposalItem } from "@loomarr/api/models/proposalItem";
import { describe, expect, it } from "vitest";
import { episodeSelectionLabel } from "./episode-selection-label";

const movie: ProposalItem = { name: "Heat", mediaType: "movie", inLibrary: true };
const series: ProposalItem = { name: "The Simpsons", mediaType: "series", inLibrary: true };

describe("episodeSelectionLabel", () => {
  it("leaves movies unlabeled", () => {
    expect(episodeSelectionLabel(movie)).toBeNull();
  });

  it("treats an omitted or unknown series selector as all episodes", () => {
    expect(episodeSelectionLabel(series)).toBe("All episodes");
    expect(episodeSelectionLabel({ ...series, episodeSelection: { mode: "retired-mode" } })).toBe(
      "All episodes",
    );
  });

  const selections: Array<[EpisodeSelection, string]> = [
    [{ mode: "complete" }, "All episodes"],
    [{ mode: "highlights" }, "Curated highlights"],
    [{ mode: "holiday", holidays: ["christmas"] }, "christmas episodes"],
    [{ mode: "holiday" }, "Holiday episodes"],
  ];

  it.each(selections)("labels %o", (selection, expected) => {
    expect(episodeSelectionLabel({ ...series, episodeSelection: selection })).toBe(expected);
  });

  it("uses a server preview only for a series without its own selector", () => {
    const preview: EpisodeSelection = { mode: "highlights" };
    expect(episodeSelectionLabel(series, preview)).toBe("Curated highlights");
    expect(episodeSelectionLabel(movie, preview)).toBeNull();
  });

  it("gives an item selector precedence over the server preview", () => {
    expect(
      episodeSelectionLabel({ ...series, episodeSelection: { mode: "complete" } }, { mode: "highlights" }),
    ).toBe("All episodes");
  });
});
