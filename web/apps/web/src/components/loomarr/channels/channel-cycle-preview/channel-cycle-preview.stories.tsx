import { ExcludedItemDTOReason, type PreviewCycleOutputBody } from "@loomarr/api";
import type { Decorator, Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { widthFrame } from "@/test/story-utils";
import { ChannelCyclePreview } from "./channel-cycle-preview";

const jsonResponse = (body: unknown) =>
  new Response(JSON.stringify(body), { status: 200, headers: { "content-type": "application/json" } });

// `nothingExcluded` — the report a healthy channel returns. Required by the contract, so it is
// spelled out rather than omitted; `ExcludedPanel` renders nothing for it.
const nothingExcluded: PreviewCycleOutputBody["excluded"] = {
  items: [],
  overCeiling: 0,
  unrated: 0,
};

const emptyTrace: PreviewCycleOutputBody["trace"] = {
  version: 1,
  ordering: "sequential",
  seed: "0",
  windowMs: 0,
  windowIndex: 0,
  relaxations: [],
  factTotal: 0,
  recordedTotal: 0,
  truncated: false,
  facts: [],
};

// The preview ignores the pass summary (the Programming page's Play order line reads it).
const emptyCycle: PreviewCycleOutputBody["cycle"] = { episodes: 0, movies: 0, series: 0, runtimeMs: 0 };

// ChannelCyclePreview owns the live generated hook (channelsApi.usePreviewChannelCycle)
// rather than taking injectable state — same shape as ChannelLineupEditor elsewhere in
// this package. Stubs `fetch` deterministically (no backend, no new dependency):
// GET /v1/channels/{id}/cycle answers with a fixed preview body regardless of `at`, since
// the gallery only snapshots the initial render (no play()).
//
// ⚠ **`body` is typed `PreviewCycleOutputBody`, NOT `unknown`, and that is load-bearing.** It was
// `unknown` when the response gained a required `excluded` field, so every stub here silently kept
// answering the OLD shape — `tsc` had nothing to compare against and stayed green while all five
// stories crashed reading `.items` off `undefined`. A hand-written response body that the compiler
// cannot check against its DTO is a fixture that can only ever drift. See #281: eight further story
// files still stub `window.fetch` with untyped bodies and carry the identical latent break.
const withStubbedPreview =
  (body: PreviewCycleOutputBody): Decorator =>
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
      excluded: nothingExcluded,
      trace: emptyTrace,
      holidayEpisodes: [],
      cycle: emptyCycle,
      slots: [
        { kind: "program", title: "Snowfall at Hollis Tower", key: "movie:tmdb:90000562", part: 0 },
        { kind: "break" },
        { kind: "program", title: "Snowbound Twins", key: "movie:tmdb:90000771", part: 0 },
        { kind: "program", title: "Snowbound Twins 2", key: "movie:tmdb:90000772", part: 2 },
        { kind: "pending", title: "Tinsel Apprentice", key: "movie:tmdb:90010719" },
      ],
    }),
  ],
};

// A multi-show kids channel: series episodes render as "Show · SxxExx — Episode" (the show
// name resolved from lineupKeys by the slot's key, the season/episode from the slot), so a
// bare episode title like "Pebble Hunt" reads as "Snorkelsaurus · S1E5 — Pebble Hunt". A movie in the mix
// (no season/episode, no lineup show name) shows just its title, proving the graceful mix.
const SeriesEpisodes: Story = {
  args: {
    channelId: "ch-1",
    lineupKeys: [
      { key: "series:tmdb:90082728", title: "Snorkelsaurus" },
      { key: "series:tmdb:90003022", title: "Pram Patrol" },
      { key: "movie:tmdb:90009297", title: "Marsh Giant" },
    ],
  },
  decorators: [
    withStubbedPreview({
      at: "2026-07-25T09:00:00Z",
      activeRule: { id: "", label: "Base policy", priority: 0, matched: false },
      windowMs: 24 * 60 * 60 * 1000,
      excluded: nothingExcluded,
      trace: emptyTrace,
      holidayEpisodes: [],
      cycle: emptyCycle,
      slots: [
        { kind: "program", title: "Pebble Hunt", key: "series:tmdb:90082728", season: 1, episode: 5 },
        { kind: "program", title: "The Blanket Fort", key: "series:tmdb:90082728", season: 2, episode: 12 },
        { kind: "break" },
        { kind: "program", title: "Bibsy's a Lefty", key: "series:tmdb:90003022", season: 2, episode: 3 },
        { kind: "program", title: "Marsh Giant", key: "movie:tmdb:90009297", part: 0 }, // movie → title only
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
      excluded: nothingExcluded,
      trace: emptyTrace,
      holidayEpisodes: [],
      cycle: emptyCycle,
      slots: [
        { kind: "program", title: "Marigold Protocol", key: "movie:tmdb:90000106" },
        { kind: "break" },
        { kind: "program", title: "Copperline Foundry S1E1", key: "series:tvdb:90081189" },
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
      activeRule: { id: "r1", label: "Weekend · Cold Relay Marathon", priority: 60, matched: true },
      windowMs: 0,
      excluded: nothingExcluded,
      trace: emptyTrace,
      holidayEpisodes: [],
      cycle: emptyCycle,
      slots: [
        ...Array.from({ length: 12 }, (_, i) => ({
          kind: "program" as const,
          title: `Cold Relay S1E${i + 1}`,
          key: `series:tvdb:90000655:1:${i + 1}`,
          part: 0,
        })),
        { kind: "break" as const },
        ...Array.from({ length: 12 }, (_, i) => ({
          kind: "program" as const,
          title: `Cold Relay S2E${i + 1}`,
          key: `series:tvdb:90000655:2:${i + 1}`,
          part: 0,
        })),
        { kind: "break" as const },
        { kind: "pending" as const, title: "Cold Relay S3E1", key: "series:tvdb:90000655:3:1" },
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
      excluded: nothingExcluded,
      trace: emptyTrace,
      holidayEpisodes: [],
      cycle: emptyCycle,
      slots: [],
    }),
  ],
};

// The exclusion report, refused by a SAFETY gate (#263). `ShieldAlert` takes `text-signal` and the
// summary names the ceiling, because the operator's next action here is about a rating — not about
// loosening a filter they chose.
//
// ⚠ Collapsed, which is all the pixel suite can see: the gallery snapshots the initial render and
// `ExcludedPanel` hardcodes the disclosure shut. The ROW CONTENT (per-reason labels, the
// safety-vs-taste colour split on each `li`) is covered in channel-cycle-preview.test.tsx instead.
// Do not add a `defaultOpen` prop to the component to make this snapshottable — that is production
// API surface existing only for a screenshot.
const ExcludedBySafety: Story = {
  decorators: [
    withStubbedPreview({
      at: "2026-07-24T14:00:00Z",
      activeRule: { id: "", label: "Base policy", priority: 0, matched: false },
      windowMs: 24 * 60 * 60 * 1000,
      excluded: {
        overCeiling: 2,
        unrated: 1,
        items: [
          {
            key: "series:tmdb:90001434",
            title: "Lunar Lemmings S2E4 — Frosty Orbit",
            reason: ExcludedItemDTOReason.over_ceiling,
          },
          { key: "movie:tmdb:90000680", title: "Brass Diner", reason: ExcludedItemDTOReason.over_ceiling },
          { key: "movie:tmdb:90099999", title: "Home Movie Reel", reason: ExcludedItemDTOReason.unrated },
        ],
      },
      trace: emptyTrace,
      holidayEpisodes: [],
      cycle: emptyCycle,
      slots: [
        { kind: "program", title: "Bibsy's a Lefty", key: "series:tmdb:90003022", season: 2, episode: 3 },
        { kind: "break" },
        { kind: "program", title: "Gizmo Garage", key: "series:tmdb:90001958", season: 1, episode: 1 },
      ],
    }),
  ],
};

// The same panel when nothing tripped a safety gate — the icon drops to muted and the summary says
// "filtered out by this channel's rules". A curation choice the operator made is not an alarm.
const ExcludedByRules: Story = {
  decorators: [
    withStubbedPreview({
      at: "2026-07-24T14:00:00Z",
      activeRule: { id: "r2", label: "Weeknights · 80s only", priority: 40, matched: true },
      windowMs: 0,
      excluded: {
        overCeiling: 0,
        unrated: 0,
        items: [
          {
            key: "movie:tmdb:90000105",
            title: "Back to the Lighthouse",
            reason: ExcludedItemDTOReason.out_of_scope,
          },
          {
            key: "movie:tmdb:90000771",
            title: "Snowbound Twins",
            reason: ExcludedItemDTOReason.out_of_season,
          },
        ],
      },
      trace: emptyTrace,
      holidayEpisodes: [],
      cycle: emptyCycle,
      slots: [
        { kind: "program", title: "Marigold Protocol", key: "movie:tmdb:90000106" },
        { kind: "break" },
        { kind: "program", title: "The Driftwood Gang", key: "movie:tmdb:90009340" },
      ],
    }),
  ],
};

export default meta;
export { BasePolicy, Empty, ExcludedByRules, ExcludedBySafety, LongList, Matched, SeriesEpisodes };
