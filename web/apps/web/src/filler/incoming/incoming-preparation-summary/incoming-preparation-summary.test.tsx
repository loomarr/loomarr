import type { IncomingPreparationDTO } from "@loomarr/api";
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { IncomingPreparationSummary, readyRangeLabel } from "./incoming-preparation-summary";

const preparation = (over: Partial<IncomingPreparationDTO> = {}): IncomingPreparationDTO => ({
  state: "estimating",
  percent: 47,
  ...over,
});

describe("IncomingPreparationSummary", () => {
  it("keeps whole preparation distinct from the current processing step", () => {
    render(
      <IncomingPreparationSummary
        preparation={preparation({
          state: "estimated",
          readyIn: { lowerSeconds: 95, upperSeconds: 230 },
        })}
      />,
    );
    expect(screen.getByText("47%")).toBeInTheDocument();
    expect(screen.getByText("About 2–4 minutes")).toBeInTheDocument();
    expect(screen.getByRole("progressbar", { name: "Clip preparation" })).toHaveAttribute(
      "aria-valuenow",
      "47",
    );
  });

  it.each([
    ["waiting", "Waiting for your help"],
    ["retrying", "Trying again"],
  ] as const)("shows %s without an active bar", (state, label) => {
    render(<IncomingPreparationSummary preparation={preparation({ state })} />);
    expect(screen.getByText(label)).toBeInTheDocument();
    expect(screen.queryByRole("progressbar")).not.toBeInTheDocument();
  });

  it("shows honest unavailable and exact Ready states", () => {
    const { rerender } = render(
      <IncomingPreparationSummary preparation={preparation({ state: "unavailable", percent: undefined })} />,
    );
    expect(screen.getByText("Preparation progress unavailable")).toBeInTheDocument();
    expect(screen.queryByText("0%")).not.toBeInTheDocument();
    rerender(<IncomingPreparationSummary preparation={preparation({ state: "ready", percent: 100 })} />);
    expect(screen.getByText("100%")).toBeInTheDocument();
  });

  it("formats ranges without seconds of false precision", () => {
    expect(readyRangeLabel(10, 45)).toBe("Less than a minute");
    expect(readyRangeLabel(60, 120)).toBe("About 1–2 minutes");
    expect(readyRangeLabel(3700, 7100)).toBe("About 1–2 hours");
  });
});
