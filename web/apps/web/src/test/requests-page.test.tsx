import { getChannelGuideMockHandler } from "@loomarr/api/msw";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { server } from "@/test/msw/server";
import { ADMIN, failed, journey, MEMBER, proposal, renderAt, stub } from "@/test/requests-harness";

// #1405 — the Requests page's entry points, driven through the real router the way Home's strip
// and a bookmark reach it. The lists, the detail and the nav badge have colocated specs under
// queue/.

afterEach(() => vi.restoreAllMocks());

// One live channel, so Home has life and its strip lists what needs someone.
const withOneChannel = () =>
  server.use(
    getChannelGuideMockHandler({
      channels: [{ airings: [], channelId: "c1", name: "Test", number: 1, pendingCount: 0, status: "live" }],
      fromMs: 0,
      toMs: 0,
    }),
  );

describe("Home's strip links to Needs you (#1659)", () => {
  it("an admin's 'requests need you' Review lands on Needs you", async () => {
    stub({ me: ADMIN, proposals: [proposal] });
    withOneChannel();
    const router = renderAt("/dashboard");

    expect(await screen.findByText("1 request needs you")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("link", { name: "Review" }));
    await waitFor(() => expect(router.state.location.pathname).toBe("/requests/needs-you"));
  });

  it("a member's failed request offers Edit and retry, and no admin items", async () => {
    stub({ me: MEMBER, journeys: [failed("j-bad")] });
    withOneChannel();
    const router = renderAt("/dashboard");

    expect(await screen.findByText("1 of your requests couldn’t be built")).toBeInTheDocument();
    expect(screen.getByText("Everything’s playing")).toBeInTheDocument();
    expect(screen.queryByText(/need you/)).not.toBeInTheDocument();
    // The strip's, not the Your requests row's: both offer it, as the mock draws.
    await userEvent.click(within(screen.getByRole("status")).getByRole("link", { name: "Edit and retry" }));
    await waitFor(() => expect(router.state.location.pathname).toBe("/requests/needs-you"));
  });
});

describe("Requests landing", () => {
  it("opens on Needs you when one of your requests failed", async () => {
    stub({ me: MEMBER, journeys: [failed("j-bad")] });
    const router = renderAt("/requests");
    await waitFor(() => expect(router.state.location.pathname).toBe("/requests/needs-you"));
  });

  it("opens on In progress when nothing is waiting on you", async () => {
    stub({ me: MEMBER, journeys: [journey("j-gen", { milestone: "generating" })] });
    const router = renderAt("/requests");
    await waitFor(() => expect(router.state.location.pathname).toBe("/requests/in-progress"));
  });

  it("offers the one next action when you have never requested a channel", async () => {
    stub({ me: MEMBER });
    renderAt("/requests/in-progress");
    expect(await screen.findByText("You haven't requested a channel yet")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Request a channel" })).toBeInTheDocument();
  });
});
