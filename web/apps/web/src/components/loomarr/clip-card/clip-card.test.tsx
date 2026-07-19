import type { ClipDTO } from "@loomarr/api";
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { ClipCard } from "./clip-card";

const base: ClipDTO = {
  name: "Sunny D — Dude!",
  kind: "commercial",
  durationMs: 30000,
  era: 1990,
  audience: "kids",
  category: "food",
  tagged: true,
  aiTagged: false,
  tunarrProgramId: "clip-test",
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

  // A tagged clip must still be editable. §10's likely tagging error is a trailer scanned
  // as a commercial — it arrives with era/audience/category filled in, so it counts as
  // "tagged" while being wrong, and kind drives pod ROLE. Gating the edit on `!tagged`
  // left precisely that clip uncorrectable from the UI.
  it("offers an edit path for an already-tagged clip", () => {
    render(<ClipCard clip={{ ...base, tagged: true }} onTag={() => {}} />);
    expect(screen.getByRole("button", { name: /edit tags/i })).toBeInTheDocument();
  });
});
