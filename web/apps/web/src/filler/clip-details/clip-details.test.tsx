import type { ClipDTO } from "@loomarr/api/models/clipDTO";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { ClipDetails } from "./clip-details";

const clip: ClipDTO = {
  hash: "detail-clip",
  name: "Candy commercial",
  kind: "commercial",
  durationMs: 30000,
  aiTagged: false,
  tagged: false,
  playCount: 0,
  playsCounted: true,
};

describe("ClipDetails", () => {
  it("does not load or reveal held media until an explicit preview gesture", async () => {
    const { container, rerender } = render(<ClipDetails clip={{ ...clip, held: true }} />);
    expect(container.querySelector("video")).toBeNull();
    await userEvent.click(screen.getByRole("button", { name: "Preview clip" }));
    expect(container.querySelector("video")).toBeInTheDocument();
    rerender(<ClipDetails clip={{ ...clip, held: true, hash: "another-held-clip" }} />);
    expect(container.querySelector("video")).toBeNull();
    expect(screen.queryByText(/Channels choose clips/)).not.toBeInTheDocument();
  });

  it("shows known facts and the exact original item, not a guessed source URL", () => {
    render(
      <ClipDetails
        clip={{
          ...clip,
          era: 1977,
          audience: "kids",
          brand: "Tootsie Pop",
          assertedTags: ["candy"],
          source: "classic-ads",
          sourceUrl: "https://archive.org/details/exact-item",
        }}
      />,
    );
    expect(screen.getByText("1977")).toBeInTheDocument();
    expect(screen.getByText("Tootsie Pop")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "View original" })).toHaveAttribute(
      "href",
      "https://archive.org/details/exact-item",
    );
    expect(screen.queryByRole("button", { name: "Edit details" })).not.toBeInTheDocument();
  });

  it("shows the registered source label instead of its canonical storage key", () => {
    render(
      <ClipDetails
        clip={{
          ...clip,
          source: "youtube:https://www.youtube.com/channel/UC123/videos",
          sourceLabel: "Friendly Channel",
        }}
      />,
    );
    expect(screen.getByText("Friendly Channel")).toBeInTheDocument();
    expect(
      screen.queryByText("youtube:https://www.youtube.com/channel/UC123/videos"),
    ).not.toBeInTheDocument();
  });

  it("keeps unknown optional facts quiet and does not infer home location", () => {
    render(<ClipDetails clip={{ ...clip, kind: "unclassified", source: "filler-dir" }} />);
    expect(screen.getByText("Your clip folder")).toBeInTheDocument();
    expect(
      screen.queryByText(/Unclassified|Untagged|Geography unknown|review required/i),
    ).not.toBeInTheDocument();
    expect(screen.queryByText("Location")).not.toBeInTheDocument();
    expect(screen.queryByRole("link")).not.toBeInTheDocument();
  });

  it.each([
    "javascript:alert(1)",
    "https://secret:password@archive.org/details/reel",
    "file:///private/movie",
    "not-a-url",
  ])("does not link an unsafe or absent source: %s", (sourceUrl) => {
    render(<ClipDetails clip={{ ...clip, source: "classic", sourceUrl }} />);
    expect(screen.queryByRole("link")).not.toBeInTheDocument();
    expect(screen.getByText("Imported source")).toBeInTheDocument();
  });

  it("does not describe preview playback as an airing", () => {
    render(<ClipDetails clip={{ ...clip, playsCounted: false }} />);
    expect(screen.getByText("Airings aren't counted on this setup")).toBeInTheDocument();
    expect(screen.queryByText("Never aired")).not.toBeInTheDocument();
  });

  it.each([
    ["adding_details", "Adding details", "checking this clip in the background"],
    ["details_limited", "Details limited", "couldn't be confirmed"],
  ] as const)("renders the server-owned %s state without making it a task", (state, title, detail) => {
    render(<ClipDetails clip={{ ...clip, enrichment: { state } }} />);
    expect(screen.getByText(title)).toBeInTheDocument();
    expect(screen.getByText(new RegExp(detail, "i"))).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /fix|retry|review/i })).not.toBeInTheDocument();
  });

  it("keeps a complete enrichment state quiet", () => {
    render(<ClipDetails clip={{ ...clip, enrichment: { state: "complete" } }} />);
    expect(screen.queryByText(/Adding details|Details limited/)).not.toBeInTheDocument();
  });

  it("labels researched campaign context as likely and links its evidence", () => {
    render(
      <ClipDetails
        clip={{
          ...clip,
          contextSuggestion: {
            decade: 1970,
            countryCode: "US",
            country: "United States",
            confidence: 70,
            explanation: "The campaign debuted then; this exact cut is not proven.",
            sources: [{ title: "Tootsie Pop", url: "https://en.wikipedia.org/wiki/Tootsie_Pop" }],
          },
        }}
      />,
    );
    expect(screen.getByText("Likely 1970s · United States")).toBeInTheDocument();
    expect(screen.getByText(/not confirmed details for this exact clip/i)).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /Tootsie Pop/ })).toHaveAttribute(
      "href",
      "https://en.wikipedia.org/wiki/Tootsie_Pop",
    );
    expect(screen.queryByText("Year")).not.toBeInTheDocument();
    expect(screen.queryByText("Location")).not.toBeInTheDocument();
  });
});
