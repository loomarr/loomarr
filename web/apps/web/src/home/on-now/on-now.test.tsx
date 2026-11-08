import type { GuideChannelTimeline } from "@loomarr/api/models/guideChannelTimeline";
import { layoutGuide } from "@loomarr/core/guide";
import { createMemoryHistory, createRootRoute, createRouter, RouterProvider } from "@tanstack/react-router";
import { screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { render } from "@/test/render";
import { OnNow } from "./on-now";

// A mounted router: On now's cards and View channel links both route through it.
const renderOnNow = async (channels: GuideChannelTimeline[], nowMs = 0, toMs = 1) => {
  const layout = layoutGuide({ channels, fromMs: 0, toMs }, nowMs);
  const rootRoute = createRootRoute({ component: () => <OnNow layout={layout} nowMs={nowMs} /> });
  const router = createRouter({
    routeTree: rootRoute,
    history: createMemoryHistory({ initialEntries: ["/"] }),
  });
  render(<RouterProvider router={router} />);
  // Not a bare role query: a card's own programme title is an <h3>, also role "heading", so an
  // unqualified query is ambiguous once a channel has a current block.
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
      stopMs: 1,
    },
  ],
  channelId: id,
  name: `Channel ${id}`,
  number,
  pendingCount: 0,
  status: "live",
  ...over,
});

describe("OnNow — up to six channels in number order (#1822 evidence)", () => {
  it("shows On now with a Full guide link once a channel has a current block", async () => {
    await renderOnNow([live("c1", 7)]);
    expect(screen.getByRole("heading", { name: "On now" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Full guide" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /Channel c1\b.*Watch live$/ })).toBeInTheDocument();
  });

  it("orders by channel number and caps at six", async () => {
    const channels = Array.from({ length: 9 }, (_, i) => live(`c${i}`, 90 - i));
    await renderOnNow(channels);
    const links = screen.getAllByRole("link", { name: /Watch live$/ });
    expect(links).toHaveLength(6);
    // Numbers 82..90 reversed by construction: lowest six numbers are 82-87.
    expect(links[0]).toHaveAccessibleName(/Channel c8\b.*Watch live$/);
  });

  it("falls back to Your channels, with actual lifecycle wording, when nothing has a current block", async () => {
    const channels: GuideChannelTimeline[] = [
      { airings: [], channelId: "p1", name: "Paused One", number: 3, pendingCount: 0, status: "paused" },
      { airings: [], channelId: "b1", name: "Building One", number: 4, pendingCount: 0, status: "building" },
      { airings: [], channelId: "g1", name: "Gap One", number: 5, pendingCount: 0, status: "live" },
    ];
    await renderOnNow(channels);
    expect(screen.getByRole("heading", { name: "Your channels" })).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "Full guide" })).not.toBeInTheDocument();
    expect(screen.getByText("Paused")).toBeInTheDocument();
    expect(screen.getByText("Building schedule")).toBeInTheDocument();
    expect(screen.getByText("No programme scheduled now")).toBeInTheDocument();
    expect(screen.getAllByRole("link", { name: /View channel/ })).toHaveLength(3);
  });

  // #2761: during a break the card was titled "Commercials · <spot>", so a busy evening read as a
  // wall of ads. It names the programme the break leads back to, with an Ad break badge.
  it("titles a channel in a break with the programme it leads back to", async () => {
    const MIN = 60_000;
    await renderOnNow(
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
      31 * MIN,
      60 * MIN,
    );
    expect(screen.getByRole("heading", { name: "Harbour Lights" })).toBeInTheDocument();
    expect(screen.getByText("Ad break")).toBeInTheDocument();
    expect(screen.queryByText(/Commercials/)).not.toBeInTheDocument();
  });

  // #2727: a channel that is all breaks while its titles download says so in one plain sentence.
  it("says a channel is waiting for its titles instead of naming a spot", async () => {
    const MIN = 60_000;
    const pod = {
      entries: [{ name: "Fizzbolt", role: "commercial" as const, durationMs: MIN, isFallbackCard: false }],
      matchLevel: "exact" as const,
      totalMs: MIN,
    };
    await renderOnNow(
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
      30_000,
      2 * MIN,
    );
    expect(screen.getByText("Commercials until titles arrive · 3 of 12 downloaded")).toBeInTheDocument();
    expect(screen.queryByText(/Fizzbolt/)).not.toBeInTheDocument();
    expect(screen.queryByText("Ad break")).not.toBeInTheDocument();
  });

  it("gives a channel with no artwork the line-motif monogram fallback, not a broken image", async () => {
    await renderOnNow([live("c1", 7, { name: "Field Notes" })]);
    // No <img> is rendered for the still: the ArtworkFallback renders a presentation div instead.
    expect(screen.queryByRole("img")).not.toBeInTheDocument();
    // Twice: the still's own fallback, and the small channel-identity mark beside the title.
    expect(screen.getAllByText("FN")).toHaveLength(2);
  });

  it("wraps a very long title instead of truncating or overflowing its card", async () => {
    const longTitle = "The remarkably long story of a lighthouse keeper and the extraordinary journey home";
    await renderOnNow([
      live("c1", 7, {
        airings: [{ kind: "program", scheduleBlockId: "b1", title: longTitle, startMs: 0, stopMs: 1 }],
      }),
    ]);
    const heading = screen.getByText(longTitle);
    expect(heading.className).toContain("break-words");
  });
});
