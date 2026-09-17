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
    expect(screen.getByText("What can play now")).not.toBeVisible();
    await user.click(screen.getByText("Schedule details"));
    expect(screen.getByText("12 playable episodes or movies from 3 titles.")).toBeVisible();
    expect(screen.getByText("Requested or kept")).not.toBeVisible();
    await user.click(screen.getByText("How this estimate works"));
    expect(screen.getByText("Requested or kept")).toBeVisible();
    expect(screen.queryByText("Other suggestions")).not.toBeInTheDocument();
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
    expect(
      screen.getByText("1 title still needs adding and isn't included in the estimate yet."),
    ).toBeVisible();
    expect(screen.queryByText(/Discovery-led/)).not.toBeInTheDocument();
  });

  it("hides previous estimates while a newer observation is pending", () => {
    render(<ProposalOutlook pending assessment={outlook()} />);
    expect(screen.getByRole("status")).toHaveTextContent("Checking this lineup");
    expect(screen.queryByText("Ready to start")).not.toBeInTheDocument();
  });

  it("distinguishes missing media from unconfirmed availability and labels a partial preview", async () => {
    render(
      <ProposalOutlook assessment={outlook({ missingLibrary: 1, unknownTitles: 2, windowLimited: true })} />,
    );
    await userEvent.click(screen.getByText("Schedule details"));
    expect(
      screen.getByText("1 library title isn't ready to play and isn't included in the estimate."),
    ).toBeVisible();
    expect(
      screen.getByText(
        "Availability hasn't been confirmed for 2 titles. They aren't included in the estimate.",
      ),
    ).toBeVisible();
    expect(screen.getByText("This is a partial preview; more from your library may fit.")).toBeVisible();
    expect(screen.queryByText(/0 acquisitions/)).not.toBeInTheDocument();
  });
});
