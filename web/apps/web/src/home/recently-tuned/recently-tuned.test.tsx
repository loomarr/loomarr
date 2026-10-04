import type { GuideChannelTimeline } from "@loomarr/api/models/guideChannelTimeline";
import { layoutGuide } from "@loomarr/core/guide";
import { createMemoryHistory, createRootRoute, createRouter, RouterProvider } from "@tanstack/react-router";
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { RecentlyTuned } from "./recently-tuned";
import type { RecentlyTunedProps } from "./recently-tuned.type";

const channel = (id: string, over: Partial<GuideChannelTimeline> = {}): GuideChannelTimeline => ({
  airings: [
    { kind: "program", scheduleBlockId: `b-${id}`, title: "The Last Observatory", startMs: 0, stopMs: 1 },
  ],
  channelId: id,
  name: "Night Signal",
  number: 7,
  pendingCount: 0,
  status: "live",
  ...over,
});

// A marker that always renders beside the card, so a test asserting the card is ABSENT still has
// something to await first — the router resolves the root route asynchronously.
const renderCard = async (
  props: Omit<RecentlyTunedProps, "layout" | "nowMs"> & { channels?: GuideChannelTimeline[] },
) => {
  const { channels = [channel("c1")], ...rest } = props;
  const layout = layoutGuide({ channels, fromMs: 0, toMs: 1 }, 0);
  const rootRoute = createRootRoute({
    component: () => (
      <div data-testid="ready">
        <RecentlyTuned {...rest} layout={layout} nowMs={0} />
      </div>
    ),
  });
  const router = createRouter({
    routeTree: rootRoute,
    history: createMemoryHistory({ initialEntries: ["/"] }),
  });
  render(<RouterProvider router={router} />);
  await screen.findByTestId("ready");
};

describe("RecentlyTuned — the return card (#1822 evidence, X2)", () => {
  it("joins what's on now, never promising saved progress", async () => {
    await renderCard({
      continueWatching: { channelId: "c1", tunedAt: "2026-09-28T00:00:00Z" },
      activeChannelIds: new Set(),
    });
    expect(screen.getByRole("heading", { name: "Recently tuned" })).toBeInTheDocument();
    expect(screen.getByText("Join what’s on now.")).toBeInTheDocument();
    expect(screen.queryByText(/resume/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/continue/i)).not.toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Watch live" })).toBeInTheDocument();
  });

  it("hides when there's no last-tuned channel", async () => {
    await renderCard({ continueWatching: undefined, activeChannelIds: new Set() });
    expect(screen.queryByRole("heading", { name: "Recently tuned" })).not.toBeInTheDocument();
  });

  it("hides when the last-tuned channel no longer exists", async () => {
    await renderCard({
      continueWatching: { channelId: "gone", tunedAt: "2026-09-28T00:00:00Z" },
      activeChannelIds: new Set(),
      channels: [channel("c1")],
    });
    expect(screen.queryByRole("heading", { name: "Recently tuned" })).not.toBeInTheDocument();
  });

  it("hides when an active own-viewing card already represents the channel", async () => {
    await renderCard({
      continueWatching: { channelId: "c1", tunedAt: "2026-09-28T00:00:00Z" },
      activeChannelIds: new Set(["c1"]),
    });
    expect(screen.queryByRole("heading", { name: "Recently tuned" })).not.toBeInTheDocument();
  });

  it("hides when the channel has nothing airing right now", async () => {
    await renderCard({
      continueWatching: { channelId: "c1", tunedAt: "2026-09-28T00:00:00Z" },
      activeChannelIds: new Set(),
      channels: [channel("c1", { airings: [] })],
    });
    expect(screen.queryByRole("heading", { name: "Recently tuned" })).not.toBeInTheDocument();
  });
});
