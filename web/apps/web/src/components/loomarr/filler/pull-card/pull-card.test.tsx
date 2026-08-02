import type { PullDTO } from "@loomarr/api";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { PullCard } from "./pull-card";

const pull = (over: Partial<PullDTO> = {}): PullDTO => ({
  id: "pull_1",
  title: "Top up the 1990s",
  reason: "Saturday Mornings falls back to bumpers.",
  proposedBy: "ada",
  status: "pending",
  estimateClips: 52,
  createdAt: "2026-08-01T12:00:00Z",
  plan: [
    {
      sourceId: "classic",
      tag: "archive",
      name: "Classic TV commercials",
      why: "Era match",
      estimateClips: 40,
      dropped: false,
    },
    {
      sourceId: "psa",
      tag: "archive",
      name: "Public service",
      why: "Break variety",
      estimateClips: 12,
      dropped: false,
    },
  ],
  ...over,
});

describe("PullCard", () => {
  // ⚠ The card's whole job is to make "nothing is downloading yet" legible. An operator who
  // thinks the fetch already started has no reason to read the plan.
  it("says nothing downloads until the pull is approved", () => {
    render(<PullCard pull={pull()} onApprove={() => {}} onDismiss={() => {}} />);

    expect(screen.getByText(/nothing downloads until you approve/i)).toBeInTheDocument();
  });

  // "Approve this" without a reason is a button, not a decision.
  it("shows why the pull was proposed", () => {
    render(<PullCard pull={pull()} onApprove={() => {}} onDismiss={() => {}} />);

    expect(screen.getByText("Saturday Mornings falls back to bumpers.")).toBeInTheDocument();
    expect(screen.getByText("Top up the 1990s")).toBeInTheDocument();
  });

  // ⚠ Estimates are rendered AS estimates. What a source yields depends on what is still there
  // and what deduplicates, so an exact-looking number becomes a bug report about a forecast.
  it("renders per-source counts as approximate", () => {
    render(<PullCard pull={pull()} onApprove={() => {}} onDismiss={() => {}} />);

    expect(screen.getByText("~40 clips")).toBeInTheDocument();
  });

  // ⚠ Dropping is held locally and sent with the decision. A per-click PATCH would make
  // "half-approved" a state the gate has to reason about.
  it("sends dropped rows with the approval, in one act", async () => {
    const onApprove = vi.fn();
    render(<PullCard pull={pull()} onApprove={onApprove} onDismiss={() => {}} />);

    await userEvent.click(screen.getByRole("button", { name: /leave public service out/i }));
    expect(onApprove).not.toHaveBeenCalled();

    await userEvent.type(screen.getByLabelText("Notes for this pull"), "no local dealers");
    await userEvent.click(screen.getByRole("button", { name: "Approve pull" }));

    expect(onApprove).toHaveBeenCalledWith({ dropSourceIds: ["psa"], note: "no local dealers" });
  });

  it("lets a dropped row be put back before committing", async () => {
    const onApprove = vi.fn();
    render(<PullCard pull={pull()} onApprove={onApprove} onDismiss={() => {}} />);

    await userEvent.click(screen.getByRole("button", { name: /leave public service out/i }));
    await userEvent.click(screen.getByRole("button", { name: /put public service back/i }));
    await userEvent.click(screen.getByRole("button", { name: "Approve pull" }));

    expect(onApprove).toHaveBeenCalledWith({ dropSourceIds: [], note: "" });
  });

  // The server refuses an all-dropped approval rather than recording one that fetched nothing.
  // Saying so before the round trip is kinder than a 409.
  it("refuses to approve once every source is left out", async () => {
    render(<PullCard pull={pull()} onApprove={() => {}} onDismiss={() => {}} />);

    await userEvent.click(screen.getByRole("button", { name: /leave classic tv commercials out/i }));
    await userEvent.click(screen.getByRole("button", { name: /leave public service out/i }));

    expect(screen.getByRole("button", { name: "Approve pull" })).toBeDisabled();
    expect(screen.getByText(/nothing to fetch. dismiss it instead/i)).toBeInTheDocument();
  });

  it("dismisses without approving", async () => {
    const onApprove = vi.fn();
    const onDismiss = vi.fn();
    render(<PullCard pull={pull()} onApprove={onApprove} onDismiss={onDismiss} />);

    await userEvent.click(screen.getByRole("button", { name: "Not now" }));

    expect(onDismiss).toHaveBeenCalledOnce();
    expect(onApprove).not.toHaveBeenCalled();
  });

  it("says a decision is in flight rather than looking inert", () => {
    render(<PullCard pull={pull()} onApprove={() => {}} onDismiss={() => {}} deciding />);

    expect(screen.getByRole("button", { name: "Starting…" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Not now" })).toBeDisabled();
  });

  // huma types every Go slice as nullable, so the generated DTO says `plan: ... | null` even
  // though the handler always sends []. Rendering must survive the null.
  it("survives a null plan", () => {
    render(
      <PullCard
        pull={{ ...pull(), plan: null as unknown as PullDTO["plan"] }}
        onApprove={() => {}}
        onDismiss={() => {}}
      />,
    );

    expect(screen.getByRole("button", { name: "Approve pull" })).toBeDisabled();
  });
});
