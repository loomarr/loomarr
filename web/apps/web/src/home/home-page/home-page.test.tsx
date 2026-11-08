import { getChannelGuideMockHandler, getGuideHighlightsMockHandler } from "@loomarr/api/msw";
import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { afterEach, describe, expect, it, vi } from "vitest";
import { server } from "@/test/msw/server";
import { ADMIN, renderAt, stub } from "@/test/requests-harness";

afterEach(() => vi.restoreAllMocks());

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
      channels: [{ airings: [], channelId: "c1", name: "Test", number: 7, pendingCount: 0, status: "live" }],
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
            airing: { kind: "program", scheduleBlockId: "b1", startMs: 0, stopMs: 1, title: "The pilot" },
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
