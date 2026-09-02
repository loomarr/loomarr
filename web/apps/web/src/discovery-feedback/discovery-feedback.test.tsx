import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { DiscoveryFeedbackControls } from "./discovery-feedback";

const event = {
  id: "feedback-1",
  targetKey: "movie:tmdb:949",
  action: "never",
  scope: "household",
  actorId: "admin",
  createdAt: "2026-09-02T12:00:00Z",
};

describe("DiscoveryFeedbackControls", () => {
  it("shows an effective household choice and can undo it", () => {
    const onClear = vi.fn();
    render(
      <DiscoveryFeedbackControls
        name="Heat"
        scope={{ scope: "household" }}
        effective={event}
        onSet={vi.fn()}
        onClear={onClear}
      />,
    );

    expect(screen.getByRole("button", { name: "Never — Heat" })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByText(/Household: Never.*future suggestions only/i)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Undo" }));
    expect(onClear).toHaveBeenCalledOnce();
  });

  it("explains inherited household state and only offers a Channel override", () => {
    const onSet = vi.fn();
    render(
      <DiscoveryFeedbackControls
        name="Heat"
        scope={{ scope: "channel", scopeId: "channel-1" }}
        effective={event}
        onSet={onSet}
        onClear={vi.fn()}
      />,
    );

    expect(screen.getByText(/Inherited household preference: Never/i)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Undo" })).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Keep — Heat" }));
    expect(onSet).toHaveBeenCalledWith("keep");
  });

  it("labels a Channel-owned choice and clears that override", () => {
    render(
      <DiscoveryFeedbackControls
        name="Heat"
        scope={{ scope: "channel", scopeId: "channel-1" }}
        effective={{ ...event, scope: "channel", scopeId: "channel-1", action: "less" }}
        onSet={vi.fn()}
        onClear={vi.fn()}
      />,
    );

    expect(screen.getByText(/This Channel: Less like this/i)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Undo" })).toBeInTheDocument();
  });
});
