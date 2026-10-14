import type { SetupCheck } from "@loomarr/api";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { ConnectStep } from "./connect-step";

const check = (ok: boolean, hint?: string): SetupCheck => ({ name: "livetv", ok, hint });

describe("ConnectStep", () => {
  it("offers the action and reports the check's failure as words", () => {
    render(
      <ConnectStep check={check(false, "Tunarr didn't answer.")} cta="Connect" onConnect={vi.fn()}>
        why this matters
      </ConnectStep>,
    );
    expect(screen.getByText("why this matters")).toBeInTheDocument();
    expect(screen.getByText("Tunarr didn't answer.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Connect" })).toBeEnabled();
  });

  it("stays available once green — the wiring is idempotent, so re-running is normal", async () => {
    const onConnect = vi.fn();
    render(<ConnectStep check={check(true)} cta="Connect" onConnect={onConnect} />);

    const button = screen.getByRole("button", { name: /run again/i });
    await userEvent.click(button);
    expect(onConnect).toHaveBeenCalled();
  });

  it("blocks double-firing while the call is in flight", () => {
    render(<ConnectStep check={check(false)} cta="Connect" onConnect={vi.fn()} isPending />);
    expect(screen.getByRole("button", { name: /working/i })).toBeDisabled();
  });

  it("surfaces a failed call through ErrorState", () => {
    render(<ConnectStep check={check(false)} cta="Connect" onConnect={vi.fn()} error={new Error("boom")} />);
    expect(screen.getByRole("alert")).toHaveTextContent("boom");
  });
});
