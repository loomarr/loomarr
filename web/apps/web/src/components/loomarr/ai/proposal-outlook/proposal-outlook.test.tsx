import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { outlook } from "@/test/fixtures/outlook";
import { ProposalOutlook } from "./proposal-outlook";

describe("ProposalOutlook", () => {
  it("explains launch, fresh time and grounded mix with counts behind disclosure", async () => {
    const user = userEvent.setup();
    render(<ProposalOutlook assessment={outlook()} />);
    expect(screen.getByText("Starts now after approval")).toBeInTheDocument();
    expect(screen.getByText(/This cycle has about 4 hours/)).toBeInTheDocument();
    expect(screen.getByText(/Mostly requested picks/)).toBeInTheDocument();
    await user.click(screen.getByText("How we estimated this"));
    expect(screen.getByText(/12 unique programs/)).toBeVisible();
    expect(screen.getByText(/Core 2 · Adjacent 1/)).toBeVisible();
  });

  it("does not invent runway while acquisitions are missing", () => {
    render(
      <ProposalOutlook
        assessment={outlook({
          state: "waiting",
          programs: 0,
          missingAcquisitions: 2,
          uniqueRuntimeMs: 0,
          firstRepeatMs: null,
        })}
      />,
    );
    expect(screen.getByText("Waiting on 2 acquisitions")).toBeInTheDocument();
    expect(screen.getByText(/can be estimated when eligible media is available/)).toBeInTheDocument();
    expect(screen.queryByText(/This cycle has/)).not.toBeInTheDocument();
  });

  it("makes thinness actionable and keeps sparse evidence a lower bound", async () => {
    const user = userEvent.setup();
    const onAddVariety = vi.fn();
    render(
      <ProposalOutlook
        onAddVariety={onAddVariety}
        assessment={outlook({
          thin: true,
          windowLimited: true,
          firstRepeatMs: null,
          uniqueRuntimeMs: 3.8 * 3_600_000,
        })}
      />,
    );
    expect(screen.getByRole("status")).toHaveTextContent("May feel repetitive");
    expect(screen.getByText("At least 3 hours of fresh programming.")).toBeInTheDocument();
    expect(screen.queryByText(/first repeat comes/)).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Add more variety" }));
    expect(onAddVariety).toHaveBeenCalledOnce();
  });

  it("keeps partial launch dependencies visible and does not invent a dominant role", () => {
    render(
      <ProposalOutlook
        assessment={outlook({
          missingAcquisitions: 1,
          mix: { core: 1, adjacent: 1, discovery: 0, unknown: 0 },
        })}
      />,
    );
    expect(screen.getByText(/Starts with available programming/)).toBeVisible();
    expect(screen.getByText(/1 acquisition still missing/)).toBeVisible();
    expect(screen.getByText(/A mix of requested and related picks/)).toBeVisible();
    expect(screen.queryByText(/Discovery-led/)).not.toBeInTheDocument();
  });

  it("hides previous estimates while a newer observation is pending", () => {
    render(<ProposalOutlook pending assessment={outlook()} />);
    expect(screen.getByRole("status")).toHaveTextContent("Checking this lineup");
    expect(screen.queryByText("Starts now after approval")).not.toBeInTheDocument();
  });
});
