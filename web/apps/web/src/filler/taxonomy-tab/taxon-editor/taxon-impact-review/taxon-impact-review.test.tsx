import type { TaxonomyImpactDTO } from "@loomarr/api/models/taxonomyImpactDTO";
import { screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { render } from "@/test/render";
import { TaxonImpactReview } from "./taxon-impact-review";

const impact = (over: Partial<TaxonomyImpactDTO> = {}): TaxonomyImpactDTO => ({
  directStoredClips: 0,
  descendantStoredClips: 0,
  affectedStoredClips: 0,
  affectedPlayableClips: 0,
  descendants: [],
  savedChannelSelections: [],
  resolverTermsAdded: [],
  resolverTermsRemoved: [],
  deleteBlocked: false,
  ...over,
});

describe("TaxonImpactReview", () => {
  it("says plainly when nothing changes", () => {
    render(<TaxonImpactReview impact={impact()} />);
    expect(screen.getByRole("heading", { name: "Review the impact" })).toBeInTheDocument();
    expect(screen.getByText("No stored clip classifications will change.")).toBeInTheDocument();
    expect(screen.getByText("No playable clips need channel eligibility checks.")).toBeInTheDocument();
    expect(screen.getByText("No saved channel selections reference this branch.")).toBeInTheDocument();
    expect(screen.getByText("Classifier wording is unchanged.")).toBeInTheDocument();
    expect(screen.queryByText(/descendants kept/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/retag/i)).not.toBeInTheDocument();
  });

  it("counts the affected clips, Channels, descendants and wording", () => {
    render(
      <TaxonImpactReview
        impact={impact({
          directStoredClips: 2,
          descendantStoredClips: 4,
          affectedStoredClips: 1_206,
          affectedPlayableClips: 5,
          descendants: [
            { slug: "cereal", label: "Cereal" },
            { slug: "snacks", label: "Snacks" },
          ],
          savedChannelSelections: [{ id: "channel-1", number: 7, name: "Saturday Morning" }],
          resolverTermsAdded: ["soda"],
          resolverTermsRemoved: ["food"],
        })}
      />,
    );
    expect(
      screen.getByText(
        /1,206 stored clips may have different inherited classification \(2 direct, 4 through/,
      ),
    ).toBeInTheDocument();
    expect(screen.getByText(/5 playable clips will be checked/)).toBeInTheDocument();
    expect(screen.getByText("Descendants kept: Cereal, Snacks")).toBeInTheDocument();
    expect(screen.getByRole("listitem")).toHaveTextContent("Channel 7: Saturday Morning");
    expect(screen.getByText("Starts resolving: soda")).toBeInTheDocument();
    expect(screen.getByText("Stops resolving: food")).toBeInTheDocument();
  });

  it("explains a blocked removal", () => {
    render(<TaxonImpactReview impact={impact({ deleteBlocked: true, directStoredClips: 3 })} />);
    expect(
      screen.getByText(/retag 3 directly assigned stored clips before removing this tag/i),
    ).toBeInTheDocument();
  });
});
