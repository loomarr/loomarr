import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { RestartNeededBanner } from "./restart-needed-banner";

describe("RestartNeededBanner", () => {
  // ⚠ Names the key. "A setting changed" leaves the operator to guess which of the
  // several they just edited is the one that needs a restart.
  it("names the setting waiting on a restart", () => {
    render(<RestartNeededBanner pendingKeys={["DATABASE_URL"]} onGoToRestart={() => {}} />);

    expect(screen.getByText("DATABASE_URL")).toBeInTheDocument();
    expect(screen.getByText(/still running the old value/i)).toBeInTheDocument();
  });

  it("lists every pending key rather than counting them", () => {
    render(<RestartNeededBanner pendingKeys={["DATABASE_URL", "LISTEN_ADDR"]} onGoToRestart={() => {}} />);
    expect(screen.getByText("DATABASE_URL, LISTEN_ADDR")).toBeInTheDocument();
  });

  // Nothing pending renders nothing — an always-present banner is one the eye learns to
  // skip, which defeats the point on the day it matters.
  it("renders nothing when no restart is needed", () => {
    const { container } = render(<RestartNeededBanner pendingKeys={[]} onGoToRestart={() => {}} />);
    expect(container).toBeEmptyDOMElement();
  });

  // ⚠ It routes to the control; it does not restart. A one-click restart from a banner
  // would skip the dialog that states the consequences.
  it("sends the operator to the control instead of restarting", async () => {
    const onGoToRestart = vi.fn();
    render(<RestartNeededBanner pendingKeys={["DATABASE_URL"]} onGoToRestart={onGoToRestart} />);

    await userEvent.click(screen.getByRole("button", { name: /restart/i }));

    expect(onGoToRestart).toHaveBeenCalledOnce();
  });
});
