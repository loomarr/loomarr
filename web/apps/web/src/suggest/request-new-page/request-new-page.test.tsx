import { getGetProposalJobMockHandler } from "@loomarr/api/msw";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { server } from "@/test/msw/server";
import { ADMIN, MEMBER, renderAt, stub } from "@/test/requests-harness";

// Request a channel (#2746): the describe → review → approve flow on its own path under Requests,
// through the real route tree.

afterEach(() => vi.restoreAllMocks());

describe("Request a channel", () => {
  beforeEach(() => {
    window.sessionStorage.clear();
    server.use(
      getGetProposalJobMockHandler({
        version: 1,
        jobId: "job-1",
        milestone: "generating",
        intent: { description: "90s action movies" },
        attempts: [{ version: 1, number: 1, status: "running", startedAt: "2026-08-22T12:00:00Z" }],
        actions: ["wait"],
        createdAt: "2026-08-22T12:00:00Z",
        updatedAt: "2026-08-22T12:00:00Z",
      }),
    );
  });

  it("is named once, for who is asking", async () => {
    stub({ me: ADMIN });
    renderAt("/requests/new");
    expect(await screen.findByRole("heading", { name: "Add a channel" })).toBeInTheDocument();
    expect(screen.getAllByText("Add a channel")).toHaveLength(1);
  });

  it("asks a member to request, not add", async () => {
    stub({ me: MEMBER });
    renderAt("/requests/new");
    expect(await screen.findByRole("heading", { name: "Request a channel" })).toBeInTheDocument();
    expect(screen.getAllByText("Request a channel")).toHaveLength(1);
  });

  // The page lives under Requests, so the nav keeps Requests lit (it used to light Guide).
  it("keeps Requests highlighted in the nav", async () => {
    stub({ me: ADMIN });
    renderAt("/requests/new");
    await screen.findByRole("heading", { name: "Add a channel" });
    const current = screen
      .getAllByRole("link")
      .filter((link) => link.getAttribute("aria-current") === "page")
      .map((link) => link.textContent);
    expect(current).toEqual(expect.arrayContaining([expect.stringMatching(/Requests/)]));
    expect(current).not.toEqual(expect.arrayContaining([expect.stringMatching(/Guide/)]));
  });

  it("closes on Escape, back to Requests", async () => {
    stub({ me: ADMIN });
    const router = renderAt("/requests/new");
    await screen.findByLabelText("Channel intent");

    await userEvent.keyboard("{Escape}");

    await waitFor(() => expect(router.state.location.pathname).not.toBe("/requests/new"));
    expect(router.state.location.pathname).toMatch(/^\/requests/);
  });

  it("closes from the Close button too", async () => {
    stub({ me: ADMIN });
    const router = renderAt("/requests/new");

    await userEvent.click(await screen.findByRole("button", { name: "Close" }));

    await waitFor(() => expect(router.state.location.pathname).not.toBe("/requests/new"));
  });

  it("resumes the Job this tab follows", async () => {
    stub({ me: ADMIN });
    window.sessionStorage.setItem("loomarr.activeProposalJob", "job-1");
    const router = renderAt("/requests/new");

    expect(await screen.findByRole("heading", { name: "Finding titles", level: 1 })).toBeInTheDocument();
    expect(router.state.location.href).toBe("/requests/new?job=job-1");
  });

  it("reopens a draft kept across the setup trip", async () => {
    stub({ me: ADMIN });
    window.sessionStorage.setItem(
      "loomarr.pendingProposalIntent",
      JSON.stringify({ description: "Saturday morning cartoons" }),
    );
    renderAt("/requests/new");

    expect(await screen.findByLabelText("Channel intent")).toHaveValue("Saturday morning cartoons");
  });

  it("opens the manual builder from ?manual=1, under one heading", async () => {
    stub({ me: ADMIN });
    renderAt("/requests/new?manual=1");

    expect(await screen.findByLabelText("Channel name")).toBeInTheDocument();
    expect(screen.getAllByRole("heading", { name: "Add a channel" })).toHaveLength(1);
  });
});
