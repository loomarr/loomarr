import type { GrowthOutputBody } from "@loomarr/api";
import type { UpdateChannelInputBody } from "@loomarr/api/models/updateChannelInputBody";
import { getChannelGrowthMockHandler } from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { describe, expect, it, vi } from "vitest";
import { channel } from "@/test/fixtures/channels";
import { server } from "@/test/msw/server";
import { render } from "@/test/render";
import { ChannelGrowth } from "./channel-growth";

vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

type Patch = Omit<UpdateChannelInputBody, "$schema">;

const harbor = { key: "series:tvdb:90000001", name: "Harbor Lights" };
const nightDesk = { key: "series:tvdb:90000002", name: "Night Desk" };
const quiet = { key: "series:tvdb:90000004", name: "Quiet Harbor" };

const growth = (over: Partial<GrowthOutputBody> = {}): GrowthOutputBody => ({
  carried: { episodes: 280, films: 0, shows: 6 },
  joined: [
    { ...harbor, batch: "growth-2026-11-18" },
    { ...nightDesk, batch: "growth-2026-11-18" },
  ],
  line: {
    fixed: false,
    genres: ["comedy"],
    odd: [],
    words: "series from 1986 to 1995, rated up to TV-PG, comedy",
  },
  off: [{ ...quiet, reason: "Too grown-up for this channel" }],
  short: { episodes: 120, films: 0, shows: 2 },
  still: "",
  target: { episodes: 400, films: 0, shows: 8 },
  waiting: 0,
  ...over,
});

// The channel endpoint with the backend's revision contract: an accepted PATCH replaces the policy and
// lineup it carries and the response is the saved channel.
const stub = (stored: ReturnType<typeof channel>, body: GrowthOutputBody) => {
  const patches: Patch[] = [];
  const growthReads = vi.fn();
  server.use(
    http.get("*/v1/channels/:id", () => HttpResponse.json(stored)),
    http.patch("*/v1/channels/:id", async ({ request }) => {
      const patch = (await request.json()) as Patch;
      patches.push(patch);
      const saved = { ...stored, revision: stored.revision + 1 };
      if (patch.policy) saved.policy = patch.policy;
      if (patch.lineup) saved.lineup = patch.lineup;
      return HttpResponse.json(saved);
    }),
    getChannelGrowthMockHandler(() => {
      growthReads();
      return body;
    }),
  );
  return { growthReads, patches };
};

const renderGrowth = (ch: ReturnType<typeof channel>) => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <ChannelGrowth channel={ch} />
    </QueryClientProvider>,
  );
};

describe("ChannelGrowth", () => {
  it("shows what the channel read says: its words, what it needs, what joined and what came off", async () => {
    const ch = channel({ lineup: [harbor, nightDesk] });
    stub(ch, growth());
    renderGrowth(ch);

    expect(await screen.findByText("series from 1986 to 1995, rated up to TV-PG, comedy")).toBeVisible();
    expect(
      screen.getByText("Needs about 120 more episodes and 2 more shows for a week without repeats."),
    ).toBeVisible();
    expect(screen.getByText("Added on November 18, 2026")).toBeVisible();
    expect(screen.getByText("Too grown-up for this channel")).toBeVisible();
  });

  it("takes a joined title off by saving the lineup without it, then reads growth again", async () => {
    const other = { key: "movie:tmdb:90000009", name: "Lantern Street" };
    const ch = channel({ lineup: [other, harbor, nightDesk] });
    const { growthReads, patches } = stub(ch, growth());
    renderGrowth(ch);

    await userEvent.click(await screen.findByRole("button", { name: "Take off Night Desk" }));

    await waitFor(() => expect(patches).toHaveLength(1));
    expect(patches[0]).toEqual({ lineup: [other, harbor], revision: 1 });
    await waitFor(() => expect(growthReads).toHaveBeenCalledTimes(2));
  });

  it("adds a taken-off title back to the end of the lineup", async () => {
    const ch = channel({ lineup: [harbor, nightDesk] });
    const { patches } = stub(ch, growth());
    renderGrowth(ch);

    await userEvent.click(await screen.findByRole("button", { name: "Add back Quiet Harbor" }));

    await waitFor(() => expect(patches).toHaveLength(1));
    expect(patches[0]?.lineup).toEqual([harbor, nightDesk, quiet]);
  });

  it("flips the keep switch and sends the whole growth object, keeping the taken-off list", async () => {
    const policy = { growth: { declined: [quiet.key, "series:tvdb:90000005"] }, midRoll: true };
    const ch = channel({ lineup: [harbor], policy });
    const { patches } = stub(ch, growth());
    renderGrowth(ch);

    await userEvent.click(await screen.findByRole("switch", { name: /Keep as it is/ }));

    await waitFor(() => expect(patches).toHaveLength(1));
    expect(patches[0]?.policy).toEqual({
      growth: { declined: [quiet.key, "series:tvdb:90000005"], keep: true },
      midRoll: true,
    });
  });

  it("switches keep off again without dropping the taken-off list", async () => {
    const policy = { growth: { declined: [quiet.key], keep: true } };
    const ch = channel({ policy });
    const { patches } = stub(ch, growth({ still: "kept" }));
    renderGrowth(ch);

    expect(await screen.findByText("Kept as it is, so nothing joins by itself.")).toBeVisible();
    await userEvent.click(screen.getByRole("switch", { name: /Keep as it is/ }));

    await waitFor(() => expect(patches).toHaveLength(1));
    expect(patches[0]?.policy).toEqual({ growth: { declined: [quiet.key], keep: false } });
  });

  it("renders nothing when growth isn't available on this install", async () => {
    const ch = channel();
    server.use(
      http.get("*/v1/channels/:id/growth", () =>
        HttpResponse.json({ title: "Growth isn't available on this install." }, { status: 501 }),
      ),
    );
    renderGrowth(ch);

    await waitFor(() => expect(screen.queryByRole("region", { name: "Grows by itself" })).toBeNull());
  });
});
