import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { LiveIndicator } from "./live-indicator";

describe("LiveIndicator", () => {
  it("shows the LIVE label", () => {
    render(
      <LiveIndicator
        state={{ mode: "live", lagSeconds: 0, viewerTimeMs: 1_000, noticeRevision: 0 }}
        onGoLive={vi.fn()}
      />,
    );
    expect(screen.getByText("Live")).toBeInTheDocument();
  });

  it("shows paused lag and offers an accessible Go Live action", async () => {
    const goLive = vi.fn();
    render(
      <LiveIndicator
        state={{ mode: "paused", lagSeconds: 23, viewerTimeMs: 1_000, noticeRevision: 0 }}
        onGoLive={goLive}
      />,
    );

    expect(screen.getByText("Paused")).toBeInTheDocument();
    expect(screen.getByText("23s behind")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Go live" }));
    expect(goLive).toHaveBeenCalledOnce();
  });

  it("shows the current lag while playing behind live", () => {
    render(
      <LiveIndicator
        state={{ mode: "behind", lagSeconds: 61, viewerTimeMs: 1_000, noticeRevision: 0 }}
        onGoLive={vi.fn()}
      />,
    );
    expect(screen.getByText("1m 1s behind")).toBeInTheDocument();
  });
});
