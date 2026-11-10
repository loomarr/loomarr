import {
  getChannelGuideMockHandler,
  getGuideHighlightsMockHandler,
  getHouseholdViewingMockHandler,
  getJobsListMockHandler,
} from "@loomarr/api/msw";
import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { afterEach, describe, expect, it, vi } from "vitest";
import { server } from "@/test/msw/server";
import { ADMIN, MEMBER, proposal, renderAt, stub } from "@/test/requests-harness";

afterEach(() => vi.restoreAllMocks());

// A programme far enough ahead to still be "next" after the fake clock runs five hours on.
const PILOT = {
  kind: "program",
  scheduleBlockId: "b1",
  startMs: Date.now() + 24 * 60 * 60_000,
  stopMs: Date.now() + 25 * 60 * 60_000,
  title: "The pilot",
} as const;

describe("HomePage (#1659)", () => {
  it("on an empty install, says nothing is on and offers the first channel", async () => {
    stub({ me: ADMIN });
    const router = renderAt("/dashboard");
    expect(await screen.findByText("Your first channel starts here")).toBeInTheDocument();
    // The sections that need something on the air stay away.
    expect(screen.queryByRole("heading", { name: "Tonight" })).not.toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Settings → This server" })).toBeInTheDocument();
    // The request page, the same one the Requests page's door opens.
    await userEvent.click(screen.getByRole("link", { name: "Add your first channel" }));
    await waitFor(() => expect(router.state.location.pathname).toBe("/requests/new"));
    expect(router.state.location.search).toEqual({});
  });

  it("lists tonight's highlights once a channel is on", async () => {
    stub({ me: ADMIN });
    server.use(
      getChannelGuideMockHandler({
        channels: [
          {
            airings: [{ ...PILOT, series: "A sci-fi anthology", season: 4 }],
            channelId: "c1",
            name: "Test",
            number: 7,
            pendingCount: 0,
            status: "live",
          },
        ],
        fromMs: 0,
        toMs: 0,
      }),
      getGuideHighlightsMockHandler({
        fromMs: 0,
        toMs: 0,
        highlights: [
          {
            airing: { ...PILOT, series: "A sci-fi anthology", season: 4 },
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

  // The guide window is part of the query key. It is pinned at mount, so minutes passing never
  // refetch it; when it slides, the refetch runs behind the page already on screen instead of
  // unmounting every section (the #1396 blank page).
  it("requests the guide once while the clock runs, and keeps its sections mounted across a slide", async () => {
    stub({ me: ADMIN });
    const guideRequests: string[] = [];
    let releaseSlide: () => void = () => undefined;
    const slideGate = new Promise<void>((resolve) => {
      releaseSlide = resolve;
    });
    const body = {
      channels: [
        { airings: [PILOT], channelId: "c1", name: "Test", number: 7, pendingCount: 0, status: "live" },
      ],
      fromMs: 0,
      toMs: 0,
    };
    server.use(
      http.get("*/v1/guide", async ({ request }) => {
        guideRequests.push(new URL(request.url).search);
        // The first answer is immediate; the slid window's is held until the test releases it.
        if (guideRequests.length > 1) await slideGate;
        return HttpResponse.json(body as never);
      }),
      getGuideHighlightsMockHandler({
        fromMs: 0,
        toMs: 0,
        highlights: [
          {
            airing: PILOT,
            channelId: "c1",
            channelName: "Test",
            channelNumber: 7,
            reason: "season_premiere",
          },
        ],
      }),
    );
    // Only the clock is faked: msw and Testing Library keep their real timers.
    vi.useFakeTimers({ toFake: ["Date", "setInterval", "clearInterval"] });
    try {
      renderAt("/dashboard");
      expect(await screen.findByRole("heading", { name: "Tonight" })).toBeInTheDocument();

      // Two hours pass: the window is still the one asked for at mount.
      await act(async () => vi.advanceTimersByTime(2 * 60 * 60_000));
      expect(guideRequests).toHaveLength(1);

      // Another hour and a half takes the clock inside 30 minutes of the window end: it slides.
      await act(async () => vi.advanceTimersByTime(90 * 60_000));
      await waitFor(() => expect(guideRequests).toHaveLength(2));
      expect(guideRequests[1]).not.toBe(guideRequests[0]);
      // The new window is still in flight, and the page has not blanked.
      expect(screen.getByRole("heading", { name: "Tonight" })).toBeInTheDocument();

      releaseSlide();
      await waitFor(() => expect(guideRequests).toHaveLength(2));
      expect(screen.getByRole("heading", { name: "Tonight" })).toBeInTheDocument();
    } finally {
      releaseSlide();
      vi.useRealTimers();
    }
  });
});

// The admin's "Needs you" line above the wall (#2785, layout B): each count a link to its page,
// gone when every count is zero, and never shown to a member.
describe("HomePage — Needs you (#2785)", () => {
  const oneChannel = getChannelGuideMockHandler({
    channels: [{ airings: [], channelId: "c1", name: "Test", number: 7, pendingCount: 0, status: "live" }],
    fromMs: 0,
    toMs: 0,
  });

  it("shows an admin what is waiting, each count linking to its page", async () => {
    stub({ me: ADMIN, proposals: [proposal] });
    server.use(
      oneChannel,
      getJobsListMockHandler({
        jobs: [
          {
            description: "",
            group: "backup",
            lastResult: "ok",
            name: "backup",
            overdue: true,
            paused: false,
            running: false,
            schedule: "0 3 * * *",
            scheduleKey: "backup.schedule",
            title: "Back up",
          },
        ],
      }),
    );
    renderAt("/dashboard");

    const line = await screen.findByRole("navigation", { name: "Needs you" });
    const approve = await within(line).findByRole("link", { name: /1 request to approve/ });
    expect(approve).toHaveAttribute("href", "/requests/needs-you");
    expect(await within(line).findByRole("link", { name: /1 task overdue/ })).toHaveAttribute(
      "href",
      "/settings/system/tasks",
    );
  });

  it("hides the line when nothing needs the admin", async () => {
    stub({ me: ADMIN });
    server.use(oneChannel);
    renderAt("/dashboard");

    expect(await screen.findByRole("heading", { name: "On now" })).toBeInTheDocument();
    expect(screen.queryByRole("navigation", { name: "Needs you" })).not.toBeInTheDocument();
  });

  it("never shows a member the line", async () => {
    stub({ me: MEMBER, proposals: [proposal] });
    server.use(oneChannel);
    renderAt("/dashboard");

    expect(await screen.findByRole("heading", { name: "On now" })).toBeInTheDocument();
    expect(screen.queryByRole("navigation", { name: "Needs you" })).not.toBeInTheDocument();
  });
});

// Names on the wall's tiles are an admin's view only (#2785, privacy decision on #3010): a member is
// not told who else is watching.
describe("HomePage — who is watching on the wall (#2785)", () => {
  const channel = { airings: [], channelId: "c1", name: "Test", number: 7, pendingCount: 0, status: "live" };
  const guide = getChannelGuideMockHandler({
    channels: [{ ...channel, status: "live" }],
    fromMs: 0,
    toMs: 0,
  });
  const sam = { channelId: "c1", device: "TV", name: "Sam", since: "2026-11-09T10:00:00Z", userId: "u1" };

  it("names the viewers on their tile for an admin", async () => {
    stub({ me: ADMIN });
    server.use(
      guide,
      getHouseholdViewingMockHandler({
        channels: [{ channelId: "c1", viewers: 1 }],
        scope: "household",
        viewers: [{ ...sam, you: false }],
        watching: 1,
      }),
    );
    renderAt("/dashboard");

    expect(await screen.findByText("Sam watching")).toBeInTheDocument();
    expect(screen.getByText("1 channel · 1 person watching")).toBeInTheDocument();
  });

  it("gives a member no names and no count on the wall", async () => {
    stub({ me: MEMBER });
    server.use(
      guide,
      getHouseholdViewingMockHandler({
        channels: [{ channelId: "c1", viewers: 1 }],
        scope: "self",
        viewers: [],
        watching: 1,
      }),
    );
    renderAt("/dashboard");

    expect(await screen.findByRole("heading", { name: "On now" })).toBeInTheDocument();
    expect(screen.getByText("1 channel")).toBeInTheDocument();
    expect(screen.queryByText(/Sam/)).not.toBeInTheDocument();
  });
});
