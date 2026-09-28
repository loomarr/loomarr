import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { RouterHarness } from "@/test/story-utils";
import { RequestCard } from "./request-card";

describe("RequestCard", () => {
  it("is one link to the request's detail, carrying its title and status line", async () => {
    render(
      <RouterHarness
        content={
          <RequestCard
            jobId="job-9"
            title="90s action night"
            line="Getting 2 titles (1 downloading, 1 waiting)"
            tone="caution"
            createdAt="2026-09-24T18:00:00Z"
          />
        }
      />,
    );
    const link = await screen.findByRole("link", { name: /90s action night/ });
    expect(link).toHaveAttribute("href", "/requests/job-9");
    expect(link).toHaveTextContent("Getting 2 titles (1 downloading, 1 waiting)");
  });

  it("says when it was requested relative to now, with the exact time machine-readable", async () => {
    render(
      <RouterHarness
        content={
          <RequestCard
            jobId="job-9"
            title="90s action night"
            line="Waiting for an admin"
            tone="suggest"
            createdAt="2026-09-24T18:00:00Z"
            now={Date.parse("2026-09-27T18:00:00Z")}
          />
        }
      />,
    );
    const when = await screen.findByText("Requested 3d ago");
    expect(when.tagName).toBe("TIME");
    expect(when).toHaveAttribute("dateTime", "2026-09-24T18:00:00Z");
  });

  it("keeps the fix action outside the detail link", async () => {
    render(
      <RouterHarness
        content={
          <RequestCard
            jobId="job-9"
            title="90s action night"
            line="The model took too long."
            tone="onair"
            createdAt="2026-09-24T18:00:00Z"
            action={<button type="button">Try again</button>}
          />
        }
      />,
    );
    const link = await screen.findByRole("link", { name: /90s action night/ });
    const action = screen.getByRole("button", { name: "Try again" });
    expect(link).not.toContainElement(action);
  });
});
