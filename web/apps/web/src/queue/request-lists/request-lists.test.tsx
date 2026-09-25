import { screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
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
        approved("j-x", "live", [{ mediaType: "movie", name: "Gave Up", tmdbId: 7, inLibrary: false }]),
      ],
      titles: { unavailable: [title("Gave Up", "unavailable", { tmdbId: 7 })] },
    });
    renderAt("/requests/in-progress");

    expect(await screen.findByText("Couldn't get 1 title")).toBeInTheDocument();
  });

  it("gives a finished request a link to its channel", async () => {
    stub({ me: MEMBER, journeys: [journey("j-done", { channel: { id: "ch-9", name: "Cartoons" } })] });
    renderAt("/requests/done");
    expect(await screen.findByRole("link", { name: "Open channel" })).toHaveAttribute(
      "href",
      "/channels/ch-9",
    );
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
    expect(screen.getByText("Try again in a moment.")).toBeInTheDocument();
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
});
