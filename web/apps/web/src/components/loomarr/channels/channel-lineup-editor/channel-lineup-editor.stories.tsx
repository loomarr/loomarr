import type { ChannelDTO, DiscoveryFeedbackDTO, SearchOutputBody } from "@loomarr/api";
import type { Decorator, Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { widthFrame } from "@/test/story-utils";
import { ChannelLineupEditor } from "./channel-lineup-editor";

// ⚠ Generic, and every call site whose body is actually READ passes its type argument explicitly
// (GH #281). The parameter stays open because this helper only serialises — it is the call sites
// that must be checked against a DTO. An unchecked body is invisible to `tsc`, so a response type
// gaining a required field leaves the stub stale, the component reads fields off `undefined`, and
// the first sign is a Playwright baseline failure that reads like a rendering regression.
const jsonResponse = <T,>(body: T) =>
  new Response(JSON.stringify(body), { status: 200, headers: { "content-type": "application/json" } });

// ChannelLineupEditor owns live generated-API hooks (useChannelLineup → the channel writer,
// plus searchApi.useSearch for the add palette and effective feedback) rather than taking injectable state —
// same shape as RefinePanel elsewhere in this package, which stubs `fetch`
// deterministically (no backend, no new dependency) for the same reason. Dispatches by
// method/path: GET /v1/search answers the add-a-title palette with a couple of
// candidates (one in-library, one not, to exercise the Pending path); GET
// /v1/channels/{id} gives the channel writer its revision, and PATCH /v1/channels/{id}, the
// commit, is held open (see below).
const withStubbedLineup = (): Decorator => (Story) => {
  window.fetch = ((url: string, init?: RequestInit) => {
    if (typeof url === "string" && url.includes("/v1/discovery/feedback")) {
      return Promise.resolve(jsonResponse<DiscoveryFeedbackDTO[]>([]));
    }
    if (typeof url === "string" && url.includes("/v1/search")) {
      return Promise.resolve(
        jsonResponse<SearchOutputBody>({
          candidates: [
            { mediaType: "movie", tmdbId: 90000106, name: "Marigold Protocol", year: 1987, inLibrary: true },
            {
              mediaType: "series",
              tvdbId: 90081189,
              name: "Copperline Foundry",
              year: 2008,
              inLibrary: false,
            },
          ],
        }),
      );
    }
    if (init?.method === "PATCH") {
      // Held open: the story has no channel page above it to re-read the saved lineup, so the
      // add is snapshotted as the channel writer's pending lineup, which is what it documents.
      return new Promise<Response>(() => {});
    }
    if (typeof url === "string" && url.includes("/v1/channels/ch-1")) {
      // The channel writer reads the channel's revision before it saves (#1899).
      return Promise.resolve(
        jsonResponse<ChannelDTO>({
          id: "ch-1",
          revision: 1,
          name: "Late Night Noir",
          number: 1,
          inAppPlayable: true,
          status: "live",
          strategy: "shuffle",
          lineup: [],
          policy: {},
          breakCount: 0,
          pendingCount: 0,
          programCount: 0,
          slotCount: 0,
        }),
      );
    }
    return Promise.resolve(jsonResponse({}));
  }) as typeof fetch;

  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return (
    <QueryClientProvider client={client}>
      <Story />
    </QueryClientProvider>
  );
};

// The manual lineup editor on a channel's detail page (editable channels, phase 3):
// add via search, remove, reorder by drag — the direct path alongside "Refine with AI".
const meta = {
  title: "Channels/ChannelLineupEditor",
  component: ChannelLineupEditor,
  args: { channelId: "ch-1" },
  decorators: [withStubbedLineup(), widthFrame(560)],
} satisfies Meta<typeof ChannelLineupEditor>;

type Story = StoryObj<typeof meta>;

// The common case: a populated lineup showing the full range of entry states — available
// (no badge, plays now), pending (added, nothing requested it), acquiring (on its way),
// and unavailable (gave up). Each state rides on the entry (LineupEntryDTO.state, server-
// derived), so this is a plain initial render — the badges are durable, not the result of
// a click, which is why the gallery can snapshot them directly (no play()). The acquiring
// entry carries a coarse downloadStatus (§18.1), so its chip reads the stage ("Downloading…")
// rather than the generic "Acquiring…" — the Seerr acquisition-status path on a lineup row.
const Populated: Story = {
  args: {
    lineup: [
      { key: "movie:tmdb:90000949", name: "The Velvet Cartographer", year: 1995, state: "available" },
      { key: "movie:tmdb:90009426", name: "The Seventh Satellite Choir", year: 1991, state: "pending" },
      {
        key: "movie:tmdb:90001892",
        name: "Return to Comet Harbor",
        year: 1983,
        state: "acquiring",
        downloadStatus: "Downloading",
      },
      { key: "movie:tmdb:90000011", name: "Comet Harbor", year: 1977, state: "unavailable" },
    ],
  },
};

// The phone column (#1896): at a 390px viewport the Programming tab gives the editor about 250px,
// which is narrower than the gallery's mobile frame, so this story pins that width itself. Each
// row stacks: title (and the series' season window) first, then the feedback controls, then
// remove, in the same order as Tab (tests/visual/lineup-layout.spec.ts). The row
// layout keys off the list's own width (a container query), so both gallery projects show the
// stacked rows here. Invented demo-catalogue titles; the series show the season inputs.
const NarrowColumn: Story = {
  decorators: [widthFrame(250)],
  args: {
    lineup: [
      { key: "series:tvdb:90000020", name: "The Wendelmeyer Family Hour", year: 1966, state: "available" },
      {
        key: "series:tvdb:90000021",
        name: "Bowling Alley Blues",
        year: 1974,
        state: "pending",
        seasonMax: 3,
      },
      { key: "movie:tmdb:90000017", name: "Glacier Cathedral", year: 2021, state: "available" },
    ],
  },
};

// An empty lineup — the dashed empty state, not an error.
const Empty: Story = { args: { lineup: [] } };

// The add flow end to end: play() opens the palette, searches, and picks the
// not-in-library candidate, so the resulting Pending badge is the actual component
// behaviour (the optimistic state seeded from the search result's inLibrary), not just a
// prop on a static entry.
const AddingATitle: Story = {
  args: {
    lineup: [{ key: "movie:tmdb:90000949", name: "The Velvet Cartographer", year: 1995, state: "available" }],
  },
  play: async ({ canvas, userEvent }) => {
    await userEvent.click(await canvas.findByRole("button", { name: /add a title/i }));
    await userEvent.type(canvas.getByLabelText("Search"), "copper");
    await userEvent.click(await canvas.findByText("Copperline Foundry"));
    await canvas.findByText("Pending");
  },
};

export default meta;
export { AddingATitle, Empty, NarrowColumn, Populated };
