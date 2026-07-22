import type { ProposalItem } from "@loomarr/api";
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { RefineReview } from "./refine-review";
import type { CurrentLineupItem } from "./refine-review.type";

const heat: ProposalItem = { name: "Heat", year: 1995, mediaType: "movie", tmdbId: 949, inLibrary: true };
const pointBreak: ProposalItem = {
  name: "Point Break",
  year: 1991,
  mediaType: "movie",
  tmdbId: 9426,
  inLibrary: true,
};
const predator: ProposalItem = {
  name: "Predator",
  year: 1987,
  mediaType: "movie",
  tmdbId: 106,
  inLibrary: true,
};
const conAir: ProposalItem = { name: "Con Air", year: 1997, mediaType: "movie", inLibrary: false };

const currentHeat: CurrentLineupItem = { name: "Heat", year: 1995, key: "movie:tmdb:949" };
const currentPointBreak: CurrentLineupItem = { name: "Point Break", year: 1991, key: "movie:tmdb:9426" };

describe("RefineReview", () => {
  it("classifies kept/added/removed by key", () => {
    render(
      <RefineReview
        proposed={[heat, predator]}
        current={[currentHeat, currentPointBreak]}
        onApply={vi.fn()}
        onDiscard={vi.fn()}
      />,
    );

    // Heat is on both sides → kept.
    expect(screen.getByText("Keeping · 1")).toBeInTheDocument();
    expect(screen.getByText("Heat")).toBeInTheDocument();

    // Predator is new → added.
    expect(screen.getByText("Adding · 1")).toBeInTheDocument();
    expect(screen.getByText("Predator")).toBeInTheDocument();

    // Point Break dropped off the proposed lineup → removed.
    expect(screen.getByText("Removing · 1")).toBeInTheDocument();
    expect(screen.getByText("Point Break")).toBeInTheDocument();
  });

  it("falls back to name+year matching when neither side has a usable id", () => {
    // No tmdbId/tvdbId on the proposed item and no key on the current row — the only
    // case where the fallback should even engage; heat/pointBreak (both real tmdbIds)
    // must never coincidentally collide with an unkeyed current row of the same name.
    // Predator (a real, keyed add) rides along so this scenario isn't indistinguishable
    // from "no changes" — it's the Keeping section specifically under test here.
    const unidentified: ProposalItem = { name: "Heat", year: 1995, mediaType: "movie", inLibrary: true };
    render(
      <RefineReview
        proposed={[unidentified, predator]}
        current={[{ name: "Heat", year: 1995 }]}
        onApply={vi.fn()}
        onDiscard={vi.fn()}
      />,
    );
    expect(screen.getByText("Keeping · 1")).toBeInTheDocument();
    expect(screen.getByText("Adding · 1")).toBeInTheDocument();
    expect(screen.queryByText(/removing/i)).not.toBeInTheDocument();
  });

  it("renders acquisitions under Adding with the downloading note, distinct from library adds", () => {
    render(
      <RefineReview
        proposed={[heat]}
        acquisitions={[conAir]}
        current={[currentHeat]}
        onApply={vi.fn()}
        onDiscard={vi.fn()}
      />,
    );
    expect(screen.getByText("Adding (needs downloading) · 1")).toBeInTheDocument();
    expect(screen.getByText("Con Air")).toBeInTheDocument();
  });

  it("shows the no-changes state and hides Apply when nothing differs", () => {
    render(
      <RefineReview
        proposed={[heat, pointBreak]}
        current={[currentHeat, currentPointBreak]}
        onApply={vi.fn()}
        onDiscard={vi.fn()}
      />,
    );
    expect(screen.getByText("No changes — your channel already matches.")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /apply changes/i })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: /discard/i })).toBeInTheDocument();
  });

  it("treats an empty current lineup as everything being added", () => {
    render(<RefineReview proposed={[heat, predator]} current={[]} onApply={vi.fn()} onDiscard={vi.fn()} />);
    expect(screen.getByText("Adding · 2")).toBeInTheDocument();
    expect(screen.queryByText(/keeping/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/removing/i)).not.toBeInTheDocument();
  });

  it("fires onApply and onDiscard", () => {
    const onApply = vi.fn();
    const onDiscard = vi.fn();
    render(
      <RefineReview
        proposed={[heat, predator]}
        current={[currentHeat]}
        onApply={onApply}
        onDiscard={onDiscard}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: /apply changes/i }));
    fireEvent.click(screen.getByRole("button", { name: /discard/i }));
    expect(onApply).toHaveBeenCalledOnce();
    expect(onDiscard).toHaveBeenCalledOnce();
  });

  it("disables both actions while busy", () => {
    render(
      <RefineReview
        proposed={[heat, predator]}
        current={[currentHeat]}
        onApply={vi.fn()}
        onDiscard={vi.fn()}
        busy
      />,
    );
    expect(screen.getByRole("button", { name: /apply changes/i })).toBeDisabled();
    expect(screen.getByRole("button", { name: /discard/i })).toBeDisabled();
  });
});
