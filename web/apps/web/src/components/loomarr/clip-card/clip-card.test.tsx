import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { ClipCard } from "./clip-card";
import type { Clip } from "./clip-card.type";

const base: Clip = {
  name: "Sunny D — Dude!",
  kind: "commercial",
  durationMs: 30000,
  era: 1990,
  audience: "kids",
  category: "food",
  tagged: true,
  aiTagged: false,
};

describe("ClipCard", () => {
  it("renders kind, era, audience chips and a sub-minute mono duration", () => {
    render(<ClipCard clip={base} />);
    expect(screen.getByText("Commercial")).toBeInTheDocument();
    expect(screen.getByText("1990s")).toBeInTheDocument();
    expect(screen.getByText("Kids")).toBeInTheDocument();
    expect(screen.getByText("30s")).toBeInTheDocument();
  });

  it("flags an untagged clip with a Tag action", () => {
    render(
      <ClipCard clip={{ ...base, tagged: false, era: undefined, audience: undefined }} onTag={() => {}} />,
    );
    expect(screen.getByText("Untagged")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /tag clip/i })).toBeInTheDocument();
  });

  it("marks AI-suggested tags and offers a confirm", () => {
    render(<ClipCard clip={{ ...base, tagged: false, aiTagged: true }} onConfirmTags={() => {}} />);
    expect(screen.getByText("AI-tagged")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /confirm tags/i })).toBeInTheDocument();
  });
});
