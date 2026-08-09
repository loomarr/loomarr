import type { ChannelTracksOutputBody } from "@loomarr/api";
import {
  getChannelPlayUrlMockHandler,
  getChannelTimelineMockHandler,
  getChannelTracksMockHandler,
} from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@/components/ui";
import { channel } from "@/test/fixtures/channels";
import { server } from "@/test/msw/server";
import { ChannelWatch } from "./channel-watch";

const makeWrapper = () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>
      <TooltipProvider>{children}</TooltipProvider>
    </QueryClientProvider>
  );
};

// stubTracks makes GET /v1/channels/:id/tracks return the given audio/subtitle tracks, and
// reports whether it was asked at all — the last test's whole claim is that it was NOT.
//
// ⚠ The stub this replaced ended in a catch-all `jsonResponse(200, {})`, so any other request the
// player made was answered with an empty object and nothing said so. It also matched on the
// substring "/tracks", which would have accepted that path under any resource.
const stubTracks = (tracks: Partial<ChannelTracksOutputBody> = {}) => {
  let probed = false;
  server.use(
    // ⚠ The player also reads the channel timeline; the OLD catch-all answered it with `{}`, so
    // the strip rendered against an empty object and nothing said so. The guard named it.
    getChannelTimelineMockHandler({ airings: [] }),
    // ⚠ And the play-url mint. THREE requests this component makes were answered by the old
    // `json({})` catch-all — timeline, play-url and any other — so three code paths ran against
    // an empty object with nothing to say so.
    getChannelPlayUrlMockHandler({
      url: "http://localhost/hls/master.m3u8",
      relativeUrl: "/hls/master.m3u8",
      expiresAt: "2026-08-09T23:59:59Z",
    }),
    getChannelTracksMockHandler(() => {
      probed = true;
      return { audio: tracks.audio ?? [], subtitles: tracks.subtitles ?? [] };
    }),
  );
  return { wasProbed: () => probed };
};

// ⚠ `as ChannelDTO` is GONE. The cast silenced eleven missing required fields — it is the exact
// escape hatch the shared `channel()` fixture exists to remove, and a component reading
// `pendingCount` off this object would have seen undefined where the server always sends a number.
const live = channel({ id: "ch-1", name: "Late Night Noir", number: 42, status: "live" });

describe("ChannelWatch pickers", () => {
  // The audio/subtitle controls live IN the player's bar now (V47), so the player must be started
  // ("Watch live") before they render — that click is also what enables the /tracks probe.
  const startWatching = async () => {
    await userEvent.click(await screen.findByRole("button", { name: /Watch .* live/ }));
  };

  it("builds the Audio menu from the AIRING media's tracks, not a hardcoded list", async () => {
    // The airing programme carries English + Russian audio — so those, and only those (plus Auto),
    // are the choices. A hardcoded list would show French/Spanish/Japanese, which this asserts absent.
    stubTracks({
      audio: [
        { index: 0, language: "eng" },
        { index: 1, language: "rus" },
      ],
    });

    render(<ChannelWatch channel={live} isAdmin onSavePolicy={vi.fn()} />, { wrapper: makeWrapper() });
    await startWatching();

    // Open the Audio menu (an icon button in the player bar). Its items are menuitemcheckboxes.
    await userEvent.click(await screen.findByRole("button", { name: "Audio" }));
    expect(await screen.findByRole("menuitemcheckbox", { name: /English/ })).toBeInTheDocument();
    expect(screen.getByRole("menuitemcheckbox", { name: /Russian/ })).toBeInTheDocument();
    expect(screen.getByRole("menuitemcheckbox", { name: /Auto/ })).toBeInTheDocument();
    // Nothing the media does not carry.
    expect(
      screen.queryByRole("menuitemcheckbox", { name: /French|Spanish|Japanese/ }),
    ).not.toBeInTheDocument();
  });

  it("offers Burn in only when the airing media has subtitle tracks", async () => {
    stubTracks({ subtitles: [] });
    const { unmount } = render(<ChannelWatch channel={live} isAdmin onSavePolicy={vi.fn()} />, {
      wrapper: makeWrapper(),
    });
    await startWatching();

    // No subtitle tracks → the Subtitles menu offers only Off (no Burn in for media with none).
    await userEvent.click(await screen.findByRole("button", { name: "Subtitles" }));
    await waitFor(() => expect(screen.getByRole("menuitemcheckbox", { name: "Off" })).toBeInTheDocument());
    expect(screen.queryByRole("menuitemcheckbox", { name: /Burn in/ })).not.toBeInTheDocument();
    unmount();
  });

  it("does not fetch tracks for a paused channel (nothing airing to probe)", async () => {
    const { wasProbed } = stubTracks();

    render(<ChannelWatch channel={channel({ status: "paused" })} isAdmin onSavePolicy={vi.fn()} />, {
      wrapper: makeWrapper(),
    });

    await screen.findByText(/off air/i);
    // ⚠ The handler simply never fires. The old form asked whether any recorded url CONTAINED
    // "/tracks" — true only of the spelling the test itself chose. And if a paused channel ever
    // did fetch something unmodelled, the unhandled-request guard now fails this test by name
    // rather than a catch-all answering it.
    expect(wasProbed()).toBe(false);
  });
});
