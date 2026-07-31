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
  playCount: 0,
  playsCounted: true,
  path: "clip-test.mp4",
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

  // ⚠ The important half of V17b. A placeholder for every frameless clip would be the wrong
  // default: on a Tunarr-backed install, or one where ffmpeg never ran, that is the ENTIRE
  // catalog, and a grid of identical grey rectangles reads as a broken page rather than an
  // absent nicety. Absence is what shipped before this phase, and it already works.
  it("renders no image when the clip has no extracted frame", () => {
    const { container } = render(<ClipCard clip={{ ...base, thumbnail: undefined }} />);
    expect(container.querySelector("img")).toBeNull();
  });

  it("renders the extracted frame when the clip has one", () => {
    const { container } = render(
      <ClipCard clip={{ ...base, path: "80s/toys/intro.mp4", thumbnail: "80s/toys/intro.jpg" }} />,
    );
    const img = container.querySelector("img");
    expect(img).not.toBeNull();
    // Built from the clip's PATH, not from `thumbnail` — the route derives the .jpg itself,
    // so passing the thumbnail path would request `intro.jpg.jpg`.
    expect(img).toHaveAttribute("src", "/v1/filler/thumb/80s/toys/intro.mp4");
    // A catalog is hundreds of cards; without this every frame is fetched on mount.
    expect(img).toHaveAttribute("loading", "lazy");
  });

  // Empty alt, deliberately: the clip's name is the very next element, so a description here
  // would have a screen reader announce the same clip twice.
  it("leaves the frame's alt empty because the name is already announced", () => {
    const { container } = render(<ClipCard clip={{ ...base, name: "Frosted Flakes", thumbnail: "a.jpg" }} />);
    expect(container.querySelector("img")).toHaveAttribute("alt", "");
    expect(screen.getByText("Frosted Flakes")).toBeInTheDocument();
  });
});
