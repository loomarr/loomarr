import type { ChannelDTO } from "@loomarr/api";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@/components/ui";
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

const jsonResponse = (status: number, body: unknown) =>
  new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });

// stubTracks makes GET /tracks return the given audio/subtitle tracks; everything else 200s empty.
const stubTracks = (tracks: { audio?: unknown[]; subtitles?: unknown[] }) => {
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string) => {
      if (String(url).includes("/tracks")) {
        return Promise.resolve(
          jsonResponse(200, { audio: tracks.audio ?? [], subtitles: tracks.subtitles ?? [] }),
        );
      }
      return Promise.resolve(jsonResponse(200, {}));
    }),
  );
};

const live: ChannelDTO = {
  id: "ch-1",
  name: "Late Night Noir",
  number: 42,
  status: "live",
} as ChannelDTO;

afterEach(() => vi.unstubAllGlobals());

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
    const fetchSpy = vi.fn((_url: string) =>
      Promise.resolve(jsonResponse(200, { audio: [], subtitles: [] })),
    );
    vi.stubGlobal("fetch", fetchSpy);

    render(
      <ChannelWatch channel={{ ...live, status: "paused" } as ChannelDTO} isAdmin onSavePolicy={vi.fn()} />,
      {
        wrapper: makeWrapper(),
      },
    );

    await screen.findByText(/off air/i);
    expect(fetchSpy.mock.calls.some((args) => String(args[0]).includes("/tracks"))).toBe(false);
  });
});
