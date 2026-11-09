import type { GuideChannelTimeline } from "@loomarr/api/models/guideChannelTimeline";
import { layoutGuide } from "@loomarr/core/guide";
import { createMemoryHistory, createRootRoute, createRouter, RouterProvider } from "@tanstack/react-router";
import { screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { render } from "@/test/render";
import { ChannelWall } from "./channel-wall";

const MIN = 60_000;

// A mounted router: every tile and the Full guide link route through it.
const renderWall = async (
  channels: GuideChannelTimeline[],
  { nowMs = 0, toMs = 1, lastChannelId }: { nowMs?: number; toMs?: number; lastChannelId?: string } = {},
) => {
  const layout = layoutGuide({ channels, fromMs: 0, toMs }, nowMs);
  const rootRoute = createRootRoute({
    component: () => <ChannelWall layout={layout} nowMs={nowMs} lastChannelId={lastChannelId} />,
  });
  const router = createRouter({
    routeTree: rootRoute,
    history: createMemoryHistory({ initialEntries: ["/"] }),
  });
  render(<RouterProvider router={router} />);
  await screen.findByRole("heading", { level: 2 });
};

const live = (
  id: string,
  number: number,
  over: Partial<GuideChannelTimeline> = {},
): GuideChannelTimeline => ({
  airings: [
    {
      kind: "program",
      scheduleBlockId: `b-${id}`,
      title: `Programme ${id}`,
      startMs: 0,
      stopMs: 30 * MIN,
    },
  ],
  channelId: id,
  name: `Channel ${id}`,
  number,
  pendingCount: 0,
  status: "live",
  ...over,
});

const tiles = () => screen.getAllByRole("link").filter((l) => l.getAttribute("href")?.startsWith("/watch/"));

describe("ChannelWall — every channel as a live tile (#2764)", () => {
  it("opens on the wall with a count and a Full guide link", async () => {
    await renderWall([live("c1", 7), live("c2", 9)]);
    expect(screen.getByRole("heading", { level: 2, name: "On now" })).toBeInTheDocument();
    expect(screen.getByText("2 channels")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Full guide" })).toBeInTheDocument();
  });

  it("shows the channel number, the programme and the time left on one tile", async () => {
    await renderWall([live("c1", 7)]);
    const tile = tiles()[0] as HTMLElement;
    expect(tile).toHaveTextContent("07");
    expect(tile).toHaveTextContent("Channel c1");
    expect(within(tile).getByRole("heading", { level: 3, name: "Programme c1" })).toBeInTheDocument();
    expect(tile).toHaveTextContent("30 min left");
    expect(tile).toHaveAttribute("href", "/watch/c1");
  });

  it("lists every channel, not just six, in number order", async () => {
    const channels = Array.from({ length: 9 }, (_, i) => live(`c${i}`, 90 - i));
    await renderWall(channels);
    expect(tiles()).toHaveLength(9);
    expect(tiles()[0]).toHaveAttribute("href", "/watch/c8");
  });

  it("puts the last watched channel first and marks it", async () => {
    await renderWall([live("c1", 3), live("c2", 5), live("c3", 8)], { lastChannelId: "c3" });
    expect(tiles().map((t) => t.getAttribute("href"))).toEqual(["/watch/c3", "/watch/c1", "/watch/c2"]);
    expect(tiles()[0]).toHaveTextContent("You were here");
    expect(screen.getAllByText("You were here")).toHaveLength(1);
  });

  // The phone wall is two columns of compact tiles, last watched first and marked "Last".
  it("lays out compact tiles on a phone", async () => {
    vi.stubGlobal("matchMedia", () => ({
      matches: false,
      addEventListener: () => {},
      removeEventListener: () => {},
    }));
    try {
      await renderWall([live("c1", 3), live("c2", 5)], { lastChannelId: "c2" });
      expect(tiles()[0]).toHaveAttribute("href", "/watch/c2");
      expect(tiles()[0]).toHaveTextContent("Last");
      expect(screen.queryByText("You were here")).not.toBeInTheDocument();
    } finally {
      vi.unstubAllGlobals();
    }
  });

  it("marks nothing when the viewer has not tuned anything", async () => {
    await renderWall([live("c1", 3), live("c2", 5)]);
    expect(screen.queryByText("You were here")).not.toBeInTheDocument();
    expect(tiles()[0]).toHaveAttribute("href", "/watch/c1");
  });

  it("says why a channel with no current block has nothing on, in lifecycle wording", async () => {
    const channels: GuideChannelTimeline[] = [
      { airings: [], channelId: "p1", name: "Paused One", number: 3, pendingCount: 0, status: "paused" },
      { airings: [], channelId: "b1", name: "Building One", number: 4, pendingCount: 0, status: "building" },
      { airings: [], channelId: "g1", name: "Gap One", number: 5, pendingCount: 0, status: "live" },
    ];
    await renderWall(channels);
    expect(screen.getByText("Paused")).toBeInTheDocument();
    expect(screen.getByText("Building schedule")).toBeInTheDocument();
    expect(screen.getByText("No programme scheduled now")).toBeInTheDocument();
    expect(tiles()).toHaveLength(3);
  });

  // #2761: during a break the tile was titled "Commercials · <spot>", so a busy evening read as a
  // wall of ads. It names the programme the break leads back to and says it is an ad break.
  it("titles a channel in a break with the programme it leads back to", async () => {
    await renderWall(
      [
        live("c1", 7, {
          airings: [
            { kind: "program", scheduleBlockId: "p1", title: "Morning Show", startMs: 0, stopMs: 30 * MIN },
            {
              kind: "filler",
              scheduleBlockId: "f1",
              title: "",
              startMs: 30 * MIN,
              stopMs: 33 * MIN,
              pod: {
                entries: [
                  { name: "Fizzbolt", role: "commercial", durationMs: 3 * MIN, isFallbackCard: false },
                ],
                matchLevel: "exact",
                totalMs: 3 * MIN,
              },
            },
            {
              kind: "program",
              scheduleBlockId: "p2",
              title: "The Lighthouse",
              series: "Harbour Lights",
              season: 1,
              episode: 4,
              startMs: 33 * MIN,
              stopMs: 60 * MIN,
            },
          ],
        }),
      ],
      { nowMs: 31 * MIN, toMs: 60 * MIN },
    );
    expect(screen.getByRole("heading", { name: "Harbour Lights" })).toBeInTheDocument();
    expect(tiles()[0]).toHaveTextContent("Ad break");
    expect(screen.queryByText(/Commercials/)).not.toBeInTheDocument();
    expect(screen.queryByText(/Fizzbolt/)).not.toBeInTheDocument();
  });

  // #2727: a channel that is all breaks while its titles download says so, in plain words.
  it("says a channel is waiting for its titles instead of naming a spot", async () => {
    const pod = {
      entries: [{ name: "Fizzbolt", role: "commercial" as const, durationMs: MIN, isFallbackCard: false }],
      matchLevel: "exact" as const,
      totalMs: MIN,
    };
    await renderWall(
      [
        live("c1", 7, {
          pendingCount: 9,
          programCount: 3,
          airings: [
            { kind: "filler", scheduleBlockId: "f1", title: "", startMs: 0, stopMs: MIN, pod },
            { kind: "filler", scheduleBlockId: "f2", title: "", startMs: MIN, stopMs: 2 * MIN, pod },
          ],
        }),
      ],
      { nowMs: 30_000, toMs: 2 * MIN },
    );
    expect(screen.getByText("Commercials until titles arrive")).toBeInTheDocument();
    expect(screen.getByText("3 of 12 downloaded")).toBeInTheDocument();
    expect(screen.getByText("1 of 1 channels are still filling up")).toBeInTheDocument();
    expect(screen.queryByText(/Fizzbolt/)).not.toBeInTheDocument();
    expect(screen.queryByText("Ad break")).not.toBeInTheDocument();
  });

  it("gives a channel with no artwork generated channel art, never a hatched tile (#2762)", async () => {
    await renderWall([live("c1", 7, { name: "Field Notes" })]);
    expect(screen.queryByRole("img")).not.toBeInTheDocument();
    const art = screen.getByRole("presentation", { hidden: true });
    expect(art.style.backgroundImage).not.toContain("repeating");
    expect(screen.getByText("FN")).toBeInTheDocument();
  });

  it("keeps a very long title whole on its tile", async () => {
    const longTitle = "The remarkably long story of a lighthouse keeper and the extraordinary journey home";
    await renderWall([
      live("c1", 7, {
        airings: [{ kind: "program", scheduleBlockId: "b1", title: longTitle, startMs: 0, stopMs: 30 * MIN }],
      }),
    ]);
    expect(screen.getByRole("heading", { level: 3, name: longTitle })).toBeInTheDocument();
  });
});
