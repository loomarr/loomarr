import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { outlook } from "@/test/fixtures/outlook";
import { ProposalOutlook } from "./proposal-outlook";

describe("ProposalOutlook", () => {
  it("shows one useful schedule sentence and keeps implementation detail secondary", async () => {
    const user = userEvent.setup();
    render(<ProposalOutlook assessment={outlook()} />);
    expect(screen.getByText("About 4 hours before this lineup repeats.")).toBeInTheDocument();
    expect(screen.queryByText("Ready to start")).not.toBeInTheDocument();
    expect(screen.getByText(/Mostly requested picks/)).not.toBeVisible();
    await user.click(screen.getByText("Schedule details"));
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
    expect(
      screen.getByText("A schedule estimate will appear as the selected titles are added."),
    ).toBeInTheDocument();
    expect(screen.queryByText(/Starts as titles/)).not.toBeInTheDocument();
  });

  it("expresses a short rotation once without creating another edit workflow", () => {
    render(
      <ProposalOutlook
        assessment={outlook({
          thin: true,
          windowLimited: true,
          firstRepeatMs: null,
          uniqueRuntimeMs: 3.8 * 3_600_000,
        })}
      />,
    );
    expect(screen.getByText("Short lineup: at least 3 hours of programming.")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Add more variety" })).not.toBeInTheDocument();
  });

  it("keeps partial launch dependencies in details rather than repeating the review summary", async () => {
    const user = userEvent.setup();
    render(
      <ProposalOutlook
        assessment={outlook({
          missingAcquisitions: 1,
          mix: { core: 1, adjacent: 1, discovery: 0, unknown: 0 },
        })}
      />,
    );
    expect(screen.queryByText(/Starts with the titles already in your library/)).not.toBeInTheDocument();
    expect(screen.queryByText(/selected title is not in your library yet/)).not.toBeInTheDocument();
    await user.click(screen.getByText("Schedule details"));
    expect(screen.getByText(/A mix of requested and related picks/)).toBeVisible();
    expect(screen.queryByText(/Discovery-led/)).not.toBeInTheDocument();
  });

  it("hides previous estimates while a newer observation is pending", () => {
    render(<ProposalOutlook pending assessment={outlook()} />);
    expect(screen.getByRole("status")).toHaveTextContent("Checking this lineup");
    expect(screen.queryByText("Ready to start")).not.toBeInTheDocument();
  });
});
