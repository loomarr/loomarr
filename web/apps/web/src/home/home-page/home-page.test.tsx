import { getChannelGuideMockHandler, getGuideHighlightsMockHandler } from "@loomarr/api/msw";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { server } from "@/test/msw/server";
import { ADMIN, renderAt, stub } from "@/test/requests-harness";

afterEach(() => vi.restoreAllMocks());

describe("HomePage (#1659)", () => {
  it("on an empty install, says nothing is on and offers the first channel", async () => {
    stub({ me: ADMIN });
    const router = renderAt("/dashboard");
    expect(await screen.findByText("Nothing on air yet")).toBeInTheDocument();
    // The sections that need something on the air stay away.
    expect(screen.queryByRole("heading", { name: "Tonight" })).not.toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Settings → This server" })).toBeInTheDocument();
    // The Guide's add panel, through the same `new` search the Requests page's door uses.
    await userEvent.click(screen.getByRole("link", { name: "Add your first channel" }));
    await waitFor(() => expect(router.state.location.pathname).toBe("/guide"));
    expect(router.state.location.search).toEqual({ new: "1" });
  });

  it("lists tonight's highlights once a channel is on", async () => {
    stub({ me: ADMIN });
    server.use(
      getChannelGuideMockHandler({
        channels: [
          { airings: [], channelId: "c1", name: "Test", number: 7, pendingCount: 0, status: "live" },
        ],
        fromMs: 0,
        toMs: 0,
      }),
      getGuideHighlightsMockHandler({
        fromMs: 0,
        toMs: 0,
        highlights: [
          {
            airing: {
              kind: "program",
              scheduleBlockId: "b1",
              series: "A sci-fi anthology",
              season: 4,
              startMs: 0,
              stopMs: 1,
              title: "The pilot",
            },
            channelId: "c1",
            channelName: "Test",
            channelNumber: 7,
            reason: "season_premiere",
          },
        ],
      }),
    );
    renderAt("/dashboard");
    expect(await screen.findByRole("heading", { name: "Tonight" })).toBeInTheDocument();
    const row = screen.getByRole("link", { name: /A sci-fi anthology/ });
    expect(row).toHaveTextContent("07");
    expect(row).toHaveTextContent("Season 4 premiere");
  });
});
