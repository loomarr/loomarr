import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { StateBadge } from "./state-badge";
import type { ProvisioningState } from "./state-badge.type";

describe("StateBadge", () => {
  it("keeps the DOM text sentence-case so a screen reader reads words, not letter-spaced shouting", () => {
    render(<StateBadge state="downloading" />);
    // The uppercase + tracking is CSS-only; the DOM text a screen reader speaks
    // stays "Downloading" (§5.3), not "DOWNLOADING" nor spelled-out letters.
    expect(screen.getByText("Downloading").textContent).toBe("Downloading");
  });

  it("covers every lifecycle state", () => {
    const states: ProvisioningState[] = [
      "wanted",
      "requested",
      "downloading",
      "available",
      "unavailable",
      "drift",
    ];
    for (const s of states) {
      const { unmount } = render(<StateBadge state={s} />);
      expect(screen.getByText(new RegExp(s, "i"))).toBeInTheDocument();
      unmount();
    }
  });
});
