import type { IncomingProcessingDTO } from "@loomarr/api";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { IncomingProcessingDetails } from "./incoming-processing-details";

const processing = (over: Partial<IncomingProcessingDTO> = {}): IncomingProcessingDTO => ({
  stages: [],
  ...over,
});

describe("IncomingProcessingDetails", () => {
  it("starts collapsed and reveals measured current-step progress without implying overall progress", async () => {
    render(
      <IncomingProcessingDetails
        processing={processing({
          stages: [
            {
              label: "Adding details",
              outcome: "in_progress",
              outcomeLabel: "In progress",
              note: "Loomarr is working on this step now.",
              at: new Date().toISOString(),
              progress: 47,
            },
          ],
        })}
      />,
    );

    expect(screen.getByText("Loomarr is working on this step now.")).not.toBeVisible();
    await userEvent.click(screen.getByText("Processing details"));
    const bar = screen.getByRole("progressbar", { name: "Current step: 47%" });
    expect(bar).toHaveAttribute("aria-valuenow", "47");
    expect(screen.getByText("47%")).toBeVisible();
    expect(
      screen.queryByText(/overall progress|percent ready|ready in|estimated time/i),
    ).not.toBeInTheDocument();
  });

  it("shows an indeterminate bar when the active stage has no real measurement", async () => {
    render(
      <IncomingProcessingDetails
        processing={processing({
          stages: [
            {
              label: "Adding details",
              outcome: "in_progress",
              outcomeLabel: "In progress",
              note: "Loomarr is working on this step now.",
              at: new Date().toISOString(),
            },
          ],
        })}
      />,
    );
    await userEvent.click(screen.getByText("Processing details"));
    const bar = screen.getByRole("progressbar", { name: "Current step in progress" });
    expect(bar).not.toHaveAttribute("aria-valuenow");
    expect(screen.getByText("Working…")).toBeVisible();
  });

  it("keeps identical skipped occurrences in order and hides them until requested", async () => {
    const skipped = {
      label: "Adding details",
      outcome: "not_needed" as const,
      outcomeLabel: "Not needed",
      note: "This step was not needed for this clip.",
      at: new Date().toISOString(),
    };
    render(
      <IncomingProcessingDetails
        processing={processing({
          stages: [
            {
              label: "Checking video",
              outcome: "finished",
              outcomeLabel: "Finished",
              note: "This step finished successfully.",
              at: new Date().toISOString(),
            },
            skipped,
            { ...skipped },
          ],
        })}
      />,
    );
    await userEvent.click(screen.getByText("Processing details"));
    expect(screen.getAllByRole("listitem")).toHaveLength(1);
    await userEvent.click(screen.getByRole("button", { name: "Show 2 skipped steps" }));
    const rows = screen.getAllByRole("listitem");
    expect(rows).toHaveLength(3);
    expect(rows.map((row) => within(row).getByText(/Checking video|Adding details/).textContent)).toEqual([
      "Checking video",
      "Adding details",
      "Adding details",
    ]);
  });

  it("keeps automatic retry visible and links to its existing diagnostic owner", async () => {
    render(
      <IncomingProcessingDetails
        processing={processing({
          attempts: 2,
          nextTryAt: new Date(Date.now() + 60_000).toISOString(),
          diagnosticsHref: "/filler/manage#diagnostics",
          stages: [
            {
              label: "Checking video",
              outcome: "retrying",
              outcomeLabel: "Trying again",
              note: "This step did not finish. Loomarr will try again automatically.",
              at: new Date().toISOString(),
            },
          ],
        })}
      />,
    );
    await userEvent.click(screen.getByText("Processing details"));
    expect(screen.getByText("Trying again")).toBeVisible();
    expect(screen.getByText("This step was tried 2 times.")).toBeVisible();
    expect(screen.getByText("Next try in 1m")).toBeVisible();
    expect(screen.getByRole("link", { name: "View diagnostics" })).toHaveAttribute(
      "href",
      "/filler/manage#diagnostics",
    );
  });

  it("renders a finished Ready history without a progress claim", async () => {
    render(
      <IncomingProcessingDetails
        processing={processing({
          stages: [
            {
              label: "Finishing",
              outcome: "finished",
              outcomeLabel: "Finished",
              note: "This step finished successfully.",
              at: new Date().toISOString(),
            },
          ],
        })}
      />,
    );
    await userEvent.click(screen.getByText("Processing details"));
    expect(screen.getByText("Finished")).toBeVisible();
    expect(screen.queryByRole("progressbar")).not.toBeInTheDocument();
  });
});
