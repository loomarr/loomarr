import type { Decorator, Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { widthFrame } from "@/test/story-utils";
import { ChannelCyclePreview } from "./channel-cycle-preview";

const jsonResponse = (body: unknown) =>
  new Response(JSON.stringify(body), { status: 200, headers: { "content-type": "application/json" } });

// ChannelCyclePreview owns the live generated hook (channelsApi.usePreviewChannelCycle)
// rather than taking injectable state — same shape as ChannelLineupEditor elsewhere in
// this package. Stubs `fetch` deterministically (no backend, no new dependency):
// GET /v1/channels/{id}/cycle answers with a fixed preview body regardless of `at`, since
// the gallery only snapshots the initial render (no play()).
const withStubbedPreview =
  (body: unknown): Decorator =>
  (Story) => {
    window.fetch = (() => Promise.resolve(jsonResponse(body))) as typeof fetch;
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    return (
      <QueryClientProvider client={client}>
        <Story />
      </QueryClientProvider>
    );
  };

const meta = {
  title: "Channels/ChannelCyclePreview",
  component: ChannelCyclePreview,
  args: { channelId: "ch-1" },
  decorators: [widthFrame(640)],
} satisfies Meta<typeof ChannelCyclePreview>;

type Story = StoryObj<typeof meta>;

// A rule matched: the attribution callout names it, plus a mix of program/pending/break
// slots (with a two-part franchise pair) to show the range at a glance.
const Matched: Story = {
  decorators: [
    withStubbedPreview({
      at: "2026-12-25T09:00:00Z",
      activeRule: { id: "r1", label: "Christmas · Marathon", priority: 60, matched: true },
      windowMs: 0,
      slots: [
        { kind: "program", title: "Die Hard", key: "movie:tmdb:562", part: 0 },
        { kind: "break" },
        { kind: "program", title: "Home Alone", key: "movie:tmdb:771", part: 0 },
        { kind: "program", title: "Home Alone 2", key: "movie:tmdb:772", part: 2 },
        { kind: "pending", title: "Elf", key: "movie:tmdb:10719" },
      ],
    }),
  ],
};

// A multi-show kids channel: series episodes render as "Show · SxxExx — Episode" (the show
// name resolved from lineupKeys by the slot's key, the season/episode from the slot), so a
// bare episode title like "Grandad" reads as "Bluey · S1E5 — Grandad". A movie in the mix
// (no season/episode, no lineup show name) shows just its title, proving the graceful mix.
const SeriesEpisodes: Story = {
  args: {
    channelId: "ch-1",
    lineupKeys: [
      { key: "series:tmdb:82728", title: "Bluey" },
      { key: "series:tmdb:3022", title: "Rugrats" },
      { key: "movie:tmdb:9297", title: "Shrek" },
    ],
  },
  decorators: [
    withStubbedPreview({
      at: "2026-07-25T09:00:00Z",
      activeRule: { id: "", label: "Base policy", priority: 0, matched: false },
      windowMs: 24 * 60 * 60 * 1000,
      slots: [
        { kind: "program", title: "Grandad", key: "series:tmdb:82728", season: 1, episode: 5 },
        { kind: "program", title: "The Sleepover", key: "series:tmdb:82728", season: 2, episode: 12 },
        { kind: "break" },
        { kind: "program", title: "Chuckie's a Lefty", key: "series:tmdb:3022", season: 2, episode: 3 },
        { kind: "program", title: "Shrek", key: "movie:tmdb:9297", part: 0 }, // movie → title only
      ],
    }),
  ],
};

// No rule matched — the base-policy fallthrough, the other half of the attribution.
const BasePolicy: Story = {
  decorators: [
    withStubbedPreview({
      at: "2026-07-24T14:00:00Z",
      activeRule: { id: "", label: "Base policy", priority: 0, matched: false },
      windowMs: 24 * 60 * 60 * 1000,
      slots: [
        { kind: "program", title: "Predator", key: "movie:tmdb:106" },
        { kind: "break" },
        { kind: "program", title: "Breaking Bad S1E1", key: "series:tvdb:81189" },
      ],
    }),
  ],
};

// A whole-run marathon resolves to many slots — this is the case the slot list's inner
// scroll (max-h-96 + .scroll-thin, styles.css) exists for. Interleaving breaks and a
// trailing pending row proves those row variants still render correctly INSIDE the
// overflowing, clipped region (not just plain program rows). Slots are index-derived so
// the snapshot is deterministic (no dates/random — the visual suite is pixel-diffed).
const LongList: Story = {
  decorators: [
    withStubbedPreview({
      at: "2026-12-25T09:00:00Z",
      activeRule: { id: "r1", label: "Weekend · TNG Marathon", priority: 60, matched: true },
      windowMs: 0,
      slots: [
        ...Array.from({ length: 12 }, (_, i) => ({
          kind: "program" as const,
          title: `The Next Generation S1E${i + 1}`,
          key: `series:tvdb:655:1:${i + 1}`,
          part: 0,
        })),
        { kind: "break" as const },
        ...Array.from({ length: 12 }, (_, i) => ({
          kind: "program" as const,
          title: `The Next Generation S2E${i + 1}`,
          key: `series:tvdb:655:2:${i + 1}`,
          part: 0,
        })),
        { kind: "break" as const },
        { kind: "pending" as const, title: "The Next Generation S3E1", key: "series:tvdb:655:3:1" },
      ],
    }),
  ],
};

// Nothing resolved for this moment — the empty state, not an error.
const Empty: Story = {
  decorators: [
    withStubbedPreview({
      at: "2026-07-24T14:00:00Z",
      activeRule: { id: "", label: "Base policy", priority: 0, matched: false },
      windowMs: 0,
      slots: [],
    }),
  ],
};

export default meta;
export { BasePolicy, Empty, LongList, Matched, SeriesEpisodes };
