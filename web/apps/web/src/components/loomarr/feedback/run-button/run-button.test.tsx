import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { RunButton } from "./run-button";

describe("RunButton", () => {
  it("runs on click when idle", async () => {
    const onRun = vi.fn();
    render(<RunButton busy={false} onRun={onRun} />);

    await userEvent.click(screen.getByRole("button", { name: "Run now" }));
    expect(onRun).toHaveBeenCalledOnce();
  });

  // aria-busy and disabled are DIFFERENT claims: disabled alone says "unavailable", which is
  // what a screen reader would announce for a control that is merely working. Both are needed,
  // and a second click must not queue a second run.
  it("announces itself busy, not merely unavailable", async () => {
    const onRun = vi.fn();
    render(<RunButton busy onRun={onRun} />);

    const button = screen.getByRole("button", { name: "Running…" });
    expect(button).toHaveAttribute("aria-busy", "true");
    expect(button).toBeDisabled();

    await userEvent.click(button);
    expect(onRun).not.toHaveBeenCalled();
  });

  it("takes custom labels for both states", () => {
    const { rerender } = render(<RunButton busy={false} onRun={vi.fn()} label="Sync" busyLabel="Syncing…" />);
    expect(screen.getByRole("button", { name: "Sync" })).toBeInTheDocument();

    rerender(<RunButton busy onRun={vi.fn()} label="Sync" busyLabel="Syncing…" />);
    expect(screen.getByRole("button", { name: "Syncing…" })).toBeInTheDocument();
  });

  // `disabled` is the caller's own reason (a job that cannot run on this backend), separate
  // from busy. It must hold even when nothing is in flight, or a disabled task becomes
  // clickable the moment its run finishes.
  it("stays disabled for the caller's own reason while idle", async () => {
    const onRun = vi.fn();
    render(<RunButton busy={false} disabled onRun={onRun} />);

    const button = screen.getByRole("button", { name: "Run now" });
    expect(button).toBeDisabled();
    expect(button).toHaveAttribute("aria-busy", "false");

    await userEvent.click(button);
    expect(onRun).not.toHaveBeenCalled();
  });
});
