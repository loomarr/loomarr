import type { GuideChannelTimeline } from "@loomarr/api/models/guideChannelTimeline";
import { layoutGuide } from "@loomarr/core/guide";
import { createMemoryHistory, createRootRoute, createRouter, RouterProvider } from "@tanstack/react-router";
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { OnNow } from "./on-now";

// A mounted router: On now's cards and View channel links both route through it.
const renderOnNow = async (channels: GuideChannelTimeline[], nowMs = 0) => {
  const layout = layoutGuide({ channels, fromMs: 0, toMs: 1 }, nowMs);
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
    expect(screen.getByRole("link", { name: "Watch Channel c1 live" })).toBeInTheDocument();
  });

  it("orders by channel number and caps at six", async () => {
    const channels = Array.from({ length: 9 }, (_, i) => live(`c${i}`, 90 - i));
    await renderOnNow(channels);
    const links = screen.getAllByRole("link", { name: /^Watch / });
    expect(links).toHaveLength(6);
    // Numbers 82..90 reversed by construction: lowest six numbers are 82-87.
    expect(links[0]).toHaveAccessibleName("Watch Channel c8 live");
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
