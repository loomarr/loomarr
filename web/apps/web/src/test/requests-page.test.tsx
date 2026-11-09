import type { ChannelIdeaDTO } from "@loomarr/api/models/channelIdeaDTO";
import { getListChannelIdeasMockHandler } from "@loomarr/api/msw";
import { screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { server } from "@/test/msw/server";
import { ADMIN, failed, journey, MEMBER, renderAt, stub } from "@/test/requests-harness";

// #1405 — the Requests page's own entry points (its landing tab and first-request empty state).
// The lists, the detail and the nav badge have colocated specs under queue/.
//
// Home no longer repeats "requests need you" or a failed request on its own strip (#1822
// evidence: "Neutral channel inventory count") — that now has its own surface, the Requests nav
// badge, with "Your requests" or "On the way" still on Home itself. There is nothing left here to
// drive through Home's strip.

afterEach(() => vi.restoreAllMocks());

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

  // #2738: /requests never opens an empty tab while another has items.
  it("opens on Done when that is the only tab with requests", async () => {
    stub({ me: MEMBER, journeys: [journey("j-done")] });
    const router = renderAt("/requests");
    await waitFor(() => expect(router.state.location.pathname).toBe("/requests/done"));
  });

  it("opens on Done for an admin when the only request is finished", async () => {
    stub({ me: ADMIN, journeys: [journey("j-done")] });
    const router = renderAt("/requests");
    await waitFor(() => expect(router.state.location.pathname).toBe("/requests/done"));
  });

  it("offers the one next action when you have never requested a channel", async () => {
    stub({ me: MEMBER });
    renderAt("/requests/in-progress");
    expect(await screen.findByText("You haven't requested a channel yet")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Request a channel" })).toBeInTheDocument();
  });
});

// #2763: Channel ideas live above the Requests list, not on a tab of their own.
describe("Requests — Channel ideas", () => {
  const idea: ChannelIdeaDTO = {
    id: "i1",
    facet: "genre",
    value: "Animation",
    keys: ["m1", "m2"],
    titles: [],
    movies: 2,
    series: 0,
    inLibrary: 2,
    name: "Rainy Day Cartoons",
    pitch: "Cartoons for a wet afternoon.",
    reason: { kind: "unaired", count: 2 },
    requested: false,
    toDownload: 0,
  };

  it("shows a member's ideas above the tabs", async () => {
    stub({ me: MEMBER, journeys: [journey("j-gen", { milestone: "generating" })] });
    server.use(getListChannelIdeasMockHandler({ ideas: [idea] }));
    renderAt("/requests/in-progress");
    const heading = await screen.findByRole("heading", { name: "Channel ideas" });
    const tabs = screen.getByRole("navigation", { name: "Requests sections" });
    expect(heading.compareDocumentPosition(tabs) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(screen.getByText("Rainy Day Cartoons")).toBeInTheDocument();
  });

  it("shows no section when there is no idea", async () => {
    stub({ me: MEMBER, journeys: [journey("j-gen", { milestone: "generating" })] });
    server.use(getListChannelIdeasMockHandler({ ideas: [] }));
    renderAt("/requests/in-progress");
    await screen.findByRole("navigation", { name: "Requests sections" });
    expect(screen.queryByRole("heading", { name: "Channel ideas" })).not.toBeInTheDocument();
  });
});
