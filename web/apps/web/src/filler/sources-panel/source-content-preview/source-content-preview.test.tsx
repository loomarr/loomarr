import type { FillerProviderDescriptionDTO } from "@loomarr/api/models/fillerProviderDescriptionDTO";
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { SourceContentPreview } from "./source-content-preview";

const provider: FillerProviderDescriptionDTO = {
  id: "registry",
  label: "Registry Name",
  previewHeading: "A few from the registry",
  fetched: true,
  listable: true,
  searchable: false,
  discoverable: true,
  openSearchable: false,
  addable: true,
};

describe("SourceContentPreview", () => {
  it("holds the example section in place while its three preview links load", () => {
    const { rerender } = render(
      <SourceContentPreview provider={provider} canonicalUrl="https://example.test/channel/videos" loading />,
    );

    expect(screen.getByRole("link", { name: /Open on Registry Name/ })).toBeVisible();
    expect(screen.getByText("A few from the registry")).toBeInTheDocument();
    expect(screen.getByRole("status")).toHaveTextContent("Loading examples…");
    expect(screen.getByRole("status").querySelector(".animate-spin")).not.toBeNull();

    rerender(
      <SourceContentPreview
        provider={provider}
        canonicalUrl="https://example.test/channel/videos"
        previewItems={[
          { title: "First", url: "https://example.test/watch?v=first" },
          { title: "Second", url: "https://example.test/watch?v=second" },
          { title: "Third", url: "https://example.test/watch?v=third" },
        ]}
      />,
    );

    expect(screen.queryByRole("status")).not.toBeInTheDocument();
    expect(screen.getAllByRole("button", { name: "Preview" })).toHaveLength(3);
  });
});
