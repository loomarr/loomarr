import type { IncomingAskDTO, IncomingReelDTO } from "@loomarr/api";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactElement } from "react";
import { describe, expect, it, vi } from "vitest";
import { RouterHarness } from "@/test/story-utils";
import { IncomingPanel } from "./incoming-panel";

// ⚠ Two render helpers, and the split is deliberate rather than untidy.
//
// Only the REEL rows contain a TanStack `Link`, which needs a RouterProvider. The harness that
// supplies one mounts ASYNC, so every query in a test using it must be findBy* — a synchronous
// getByText runs before the router has rendered and fails with "unable to find", which reads as
// a component bug and is not one (coverage-meter's tests record the same trap, and this file's
// first draft walked straight into it).
//
// Ask-only tests contain no Link, so they render plainly and can assert synchronously. Wrapping
// them in the harness too would make every assertion in the file async for no reason.
const renderAsks = (ui: ReactElement) => render(ui);
const renderReels = (ui: ReactElement) => render(<RouterHarness content={ui} initialPath="/filler" />);

const guessed: IncomingAskDTO = {
  path: "1988/toys.mp4",
  name: "toys.mp4",
  from: "archive",
  durationMs: 30_000,
  kind: "commercial",
  audience: "kids",
  category: "toys",
  suggestedEra: 1988,
  reason: "The year isn't written anywhere in this clip's name or description, so Loomarr guessed it.",
};

const untagged: IncomingAskDTO = {
  path: "mystery.mp4",
  name: "mystery.mp4",
  durationMs: 25_000,
  kind: "commercial",
  reason: "Loomarr couldn't work out what this is, so it will only match broadly.",
};

const reel: IncomingReelDTO = {
  proposalId: "sp_1",
  clipPath: "comps/1987.mp4",
  segments: 12,
  needsAttention: 3,
  createdAt: "2026-08-01T12:00:00Z",
};

describe("IncomingPanel", () => {
  // ⚠ The two asks are different QUESTIONS and get different affordances. A guessed era has a
  // proposed answer to confirm; an untagged clip has nothing to confirm, so offering "Looks
  // right" there would ask an operator to approve something they were never shown.
  it("offers confirm only for a clip that carries a guess", () => {
    renderAsks(
      <IncomingPanel asks={[guessed, untagged]} reels={[]} onConfirmEra={() => {}} onEditTags={() => {}} />,
    );

    expect(screen.getAllByRole("button", { name: "Looks right" })).toHaveLength(1);
    expect(screen.getByRole("button", { name: "Not right" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Add tags" })).toBeInTheDocument();
  });

  it("shows the guess as a guess, distinct from a confirmed tag", () => {
    renderAsks(<IncomingPanel asks={[guessed]} reels={[]} />);

    expect(screen.getByText("guessed 1988")).toBeInTheDocument();
    expect(screen.getByText("kids")).toBeInTheDocument();
  });

  // ⚠ There is no confidence bar, and there must not be one until something measures it. The
  // reason the server derived is what an operator gets instead.
  it("explains why each clip is waiting, and shows no confidence score", () => {
    const { container } = renderAsks(<IncomingPanel asks={[guessed, untagged]} reels={[]} />);

    expect(screen.getByText(guessed.reason)).toBeInTheDocument();
    expect(screen.getByText(untagged.reason)).toBeInTheDocument();
    expect(container.textContent).not.toMatch(/\d+%/);
  });

  it("names the source so a bad one is identifiable across a long queue", () => {
    renderAsks(<IncomingPanel asks={[guessed]} reels={[]} />);

    expect(screen.getByText("from archive")).toBeInTheDocument();
  });

  it("passes the whole clip to the handlers, not just its path", async () => {
    const onConfirmEra = vi.fn();
    renderAsks(<IncomingPanel asks={[guessed]} reels={[]} onConfirmEra={onConfirmEra} />);

    await userEvent.click(screen.getByRole("button", { name: "Looks right" }));

    // The caller needs audience + category to build a safe PATCH: the server writes all three
    // tag columns unconditionally, so a confirm carrying only the era wipes the other two.
    expect(onConfirmEra).toHaveBeenCalledWith(guessed);
  });

  // One row disables, not the whole list — a page that greys out entirely while a single
  // confirm lands reads as having frozen.
  it("disables only the row being written", () => {
    renderAsks(
      <IncomingPanel asks={[guessed, untagged]} reels={[]} busyPath={guessed.path} onEditTags={() => {}} />,
    );

    expect(screen.getByRole("button", { name: "Not right" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Add tags" })).toBeEnabled();
  });

  it("says how much work a compilation is before it is opened", async () => {
    renderReels(<IncomingPanel asks={[]} reels={[reel]} />);

    expect(await screen.findByText("comps/1987.mp4")).toBeInTheDocument();
    expect(screen.getByText("12 clips found · 3 need a look")).toBeInTheDocument();
  });

  it("links a compilation to its review route", async () => {
    renderReels(<IncomingPanel asks={[]} reels={[reel]} />);

    const link = await screen.findByRole("link", { name: "Review cuts" });
    expect(link).toHaveAttribute("href", "/filler/splits/sp_1");
  });

  it("says nothing needs you when both halves are empty", () => {
    renderAsks(<IncomingPanel asks={[]} reels={[]} />);

    expect(screen.getByText("Nothing needs you")).toBeInTheDocument();
  });
});
