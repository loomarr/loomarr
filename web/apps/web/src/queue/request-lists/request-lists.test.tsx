import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { afterEach, describe, expect, it, vi } from "vitest";
import { server } from "@/test/msw/server";
import {
  ADMIN,
  approved,
  failed,
  journey,
  MEMBER,
  proposal,
  renderAt,
  stub,
  title,
} from "@/test/requests-harness";

afterEach(() => vi.restoreAllMocks());

describe("Requests tabs", () => {
  it("sorts each request into one tab and counts them", async () => {
    stub({
      me: MEMBER,
      journeys: [
        failed("j-bad"),
        journey("j-gen", { milestone: "generating" }),
        journey("j-wait", { milestone: "awaiting_approval" }),
        journey("j-done"),
      ],
    });
    renderAt("/requests/in-progress");

    const inProgress = await screen.findByRole("link", { name: /In progress/ });
    await waitFor(() => expect(inProgress).toHaveTextContent(/In progress\s*2\b/));
    expect(screen.getByRole("link", { name: /Needs you/ })).toHaveTextContent(/Needs you\s*1\b/);
    expect(screen.getByRole("link", { name: /Done/ })).toHaveTextContent(/Done\s*1\b/);
    expect(screen.getByText("Request j-gen")).toBeInTheDocument();
    expect(screen.queryByText("Request j-done")).not.toBeInTheDocument();
  });

  it("describes a given-up title as given up, not as queued", async () => {
    stub({
      me: MEMBER,
      journeys: [
        approved("j-x", "live", [
          { mediaType: "movie", name: "Gave Up", tmdbId: 90000007, inLibrary: false },
        ]),
      ],
      titles: { unavailable: [title("Gave Up", "unavailable", { tmdbId: 90000007 })] },
    });
    renderAt("/requests/in-progress");

    expect(await screen.findByText("Couldn't get 1 title")).toBeInTheDocument();
  });

  // #1817 Q11: a finished request is a row with two destinations, its detail (the title) and the
  // channel it became (the one trailing link).
  it("gives a finished request a link to its channel and keeps its detail link", async () => {
    stub({ me: MEMBER, journeys: [journey("j-done", { channel: { id: "ch-9", name: "Cartoons" } })] });
    renderAt("/requests/done");
    expect(await screen.findByRole("link", { name: "Open channel for Request j-done" })).toHaveAttribute(
      "href",
      "/channels/ch-9",
    );
    expect(screen.getByRole("link", { name: "Request j-done" })).toHaveAttribute("href", "/requests/j-done");
  });

  // #1817 Q11: In progress is one list of rows, not a stack of cards. Each row keeps its status and
  // a link to its detail, named for its own request so a screen reader can tell the links apart.
  it("lists in-progress requests as rows, each with its status and its own detail link", async () => {
    stub({
      me: MEMBER,
      journeys: [
        journey("j-gen", { milestone: "generating" }),
        journey("j-wait", { milestone: "awaiting_approval" }),
      ],
    });
    renderAt("/requests/in-progress");

    const list = await screen.findByRole("list", { name: "Requests in progress" });
    const rows = within(list).getAllByRole("listitem");
    expect(rows).toHaveLength(2);
    rows.forEach((row, i) => {
      const id = ["j-gen", "j-wait"][i];
      expect(within(row).getByRole("link", { name: `Open Request ${id}` })).toHaveAttribute(
        "href",
        `/requests/${id}`,
      );
      expect(within(row).getByText(/^Requested /)).toBeInTheDocument();
    });
    expect(within(list).getByText("Waiting for approval")).toBeInTheDocument();
  });

  // Deleting a channel is deliberate: it is Done with a short badge, not something the viewer must fix.
  it("files an approved request whose channel was deleted under Done, not Needs you", async () => {
    stub({
      me: MEMBER,
      journeys: [
        {
          ...approved("j-gone", "failed"),
          failure: {
            code: "generation_failed",
            reason: "generation_failed",
            recoveryAction: "retry_later",
            message: "This request was approved, but its channel no longer exists.",
            guidance: "Try again to build a new channel from this request.",
          },
        },
      ],
    });
    renderAt("/requests/done");

    expect(await screen.findByText("Channel removed")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /Needs you/ })).toHaveTextContent(/Needs you\s*0?$/);
    expect(screen.queryByTestId("nav-badge-/requests")).not.toBeInTheDocument();
  });

  it("keeps the badge a short label and the sentence in the explanation line", async () => {
    stub({ me: MEMBER, journeys: [failed("j-bad")] });
    renderAt("/requests/needs-you");

    expect(await screen.findByText("Couldn't build")).toBeInTheDocument();
    expect(screen.getByText(/The model took too long\./)).toBeInTheDocument();
  });

  it("does not repeat the guidance when the message already ends with it", async () => {
    stub({
      me: MEMBER,
      journeys: [
        {
          ...failed("j-bad"),
          failure: {
            code: "generation_failed",
            reason: "generation_failed",
            recoveryAction: "retry_later",
            message: "Loomarr couldn't finish this channel. Try again later.",
            guidance: "Try again later.",
          },
        },
      ],
    });
    renderAt("/requests/needs-you");

    const hint = await screen.findByText(/couldn't finish this channel/);
    expect(hint).toHaveTextContent(/^Loomarr couldn't finish this channel\. Try again later\.$/);
  });

  it("offers the one next action on an empty tab", async () => {
    stub({ me: MEMBER, journeys: [journey("j-done")] });
    renderAt("/requests/in-progress");
    expect(await screen.findByText("Nothing in progress")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Request a channel" })).toBeInTheDocument();
  });
});

describe("Members on Requests", () => {
  it("see all three tabs", async () => {
    stub({ me: MEMBER, journeys: [journey("j-gen", { milestone: "generating" })] });
    renderAt("/requests/in-progress");

    await screen.findByRole("link", { name: /In progress/ });
    expect(screen.getByRole("link", { name: /Needs you/ })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /Done/ })).toBeInTheDocument();
  });

  it("see their failed request with its reason and one way to fix it, never approvals", async () => {
    const seen = stub({ me: MEMBER, journeys: [failed("j-bad")], proposals: [proposal] });
    renderAt("/requests/needs-you");

    expect(await screen.findByText("Couldn't be built")).toBeInTheDocument();
    expect(screen.getByText(/Try again in a moment\./)).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Try again" })).toBeInTheDocument();
    expect(screen.queryByText("Waiting for your approval")).not.toBeInTheDocument();
    expect(seen.some((u) => /status=submitted/.test(u))).toBe(false);
  });
});

describe("Admins on Requests", () => {
  it("see approvals waiting on them, counted in Needs you", async () => {
    stub({ me: ADMIN, proposals: [proposal], pulls: [{ id: "pull1", status: "pending", items: [] }] });
    renderAt("/requests/needs-you");

    expect(await screen.findByText("Waiting for your approval")).toBeInTheDocument();
    const tab = screen.getByRole("link", { name: /Needs you/ });
    await waitFor(() => expect(tab).toHaveTextContent(/Needs you\s*2\b/));
  });

  // Found live (#1876): bulk-approving the LAST pending request dropped the count to zero, the
  // page unmounted the queue, and the result dialog — failures and all — vanished unread.
  it("keep the bulk result on screen after approving the last pending request", async () => {
    // Every real request has a job, so the page has a journey for it before and after approval.
    stub({ me: ADMIN, proposals: [proposal], journeys: [journey("j1", { milestone: "awaiting_approval" })] });
    let pending = [proposal];
    server.use(
      http.get("*/v1/proposals", ({ request }) =>
        HttpResponse.json({
          proposals: new URL(request.url).searchParams.get("status") === "submitted" ? pending : [],
        } as never),
      ),
      http.post("*/v1/proposals/approve", () => {
        pending = [];
        return HttpResponse.json({ approved: 1, results: [{ id: "p1", ok: true, enqueued: 0 }] } as never);
      }),
    );
    renderAt("/requests/needs-you");

    const group = await screen.findByRole("group", { name: "Channel requests" });
    await userEvent.click(
      within(group).getByRole("checkbox", { name: "Select Saturday cartoons for bulk approve" }),
    );
    const bulkApprove = { name: "Approve 1 selected in channel requests" };
    await waitFor(() => expect(within(group).getByRole("button", bulkApprove)).toBeEnabled());
    await userEvent.click(within(group).getByRole("button", bulkApprove));
    await userEvent.click(
      within(await screen.findByRole("dialog")).getByRole("button", { name: "Approve 1" }),
    );

    const dialog = await screen.findByRole("dialog", { name: "Approved 1 of 1" });
    // The queue has emptied underneath it; the result must still be there to read.
    expect(await screen.findByText("Queue's clear")).toBeInTheDocument();
    expect(dialog).toBeInTheDocument();

    await userEvent.click(within(dialog).getByRole("button", { name: "Done" }));
    expect(await screen.findByText("Nothing needs you")).toBeInTheDocument();
  }, 20_000);
});

// #1430: the server KEEPS a decided pull (approved or dismissed) — exactly so History can answer
// what was agreed to — but no screen asked for anything but the pending ones. Done must show them.
describe("Decided filler pulls in Done (#1430)", () => {
  const decidedPull = (id: string, title: string, status: "approved" | "dismissed") => ({
    id,
    title,
    status,
    decidedAt: "2026-09-20T10:00:00Z",
    reason: "",
    proposedBy: "",
    estimateClips: 10,
    candidateCount: 1,
    createdAt: "2026-09-01T00:00:00Z",
    rejected: [],
    sources: [],
    plan: [],
  });

  it("shows a decided pull in Done with its decision, and leaves a pending one out", async () => {
    stub({
      me: ADMIN,
      pulls: [
        decidedPull("pull-approved", "Top up the 1990s", "approved"),
        decidedPull("pull-dismissed", "Retro ads", "dismissed"),
        { ...decidedPull("pull-pending", "Still waiting", "approved"), status: "pending" },
      ],
    });
    renderAt("/requests/done");

    expect(await screen.findByText("Top up the 1990s")).toBeInTheDocument();
    expect(screen.getByText("Downloaded")).toBeInTheDocument();
    expect(screen.getByText("Retro ads")).toBeInTheDocument();
    expect(screen.getByText("Declined")).toBeInTheDocument();
    expect(screen.queryByText("Still waiting")).not.toBeInTheDocument();
  });

  // Filler pulls are an admin-only list server-side (§10 V35); a member's Done tab must not
  // show one even if it is decided.
  it("never shows a decided pull on a member's Done tab", async () => {
    stub({ me: MEMBER, pulls: [decidedPull("pull-approved", "Top up the 1990s", "approved")] });
    renderAt("/requests/done");

    expect(await screen.findByText("Nothing finished yet")).toBeInTheDocument();
    expect(screen.queryByText("Top up the 1990s")).not.toBeInTheDocument();
  });
});
