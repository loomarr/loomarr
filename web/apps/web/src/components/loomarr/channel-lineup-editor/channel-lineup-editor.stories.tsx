import type { Decorator, Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { widthFrame } from "@/test/story-utils";
import { ChannelLineupEditor } from "./channel-lineup-editor";

const jsonResponse = (body: unknown) =>
  new Response(JSON.stringify(body), { status: 200, headers: { "content-type": "application/json" } });

// ChannelLineupEditor owns live generated-API hooks (useChannelLineup → useUpdateChannel,
// plus searchApi.useSearch for the add palette) rather than taking injectable state —
// same shape as RefinePanel elsewhere in this package, which stubs `fetch`
// deterministically (no backend, no new dependency) for the same reason. Dispatches by
// method/path: GET /v1/search answers the add-a-title palette with a couple of
// candidates (one in-library, one not, to exercise the Pending path); PATCH
// /v1/channels/{id} is the commit, answered with a flat 200 since the gallery only
// snapshots the initial render (no play()) and never observes the result of a click.
const withStubbedLineup = (): Decorator => (Story) => {
  window.fetch = ((url: string, init?: RequestInit) => {
    if (typeof url === "string" && url.includes("/v1/search")) {
      return Promise.resolve(
        jsonResponse({
          candidates: [
            { mediaType: "movie", tmdbId: 106, name: "Predator", year: 1987, inLibrary: true },
            { mediaType: "series", tvdbId: 81189, name: "Breaking Bad", year: 2008, inLibrary: false },
          ],
        }),
      );
    }
    if (init?.method === "PATCH") {
      return Promise.resolve(jsonResponse({ id: "ch-1" }));
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
  title: "Loomarr/ChannelLineupEditor",
  component: ChannelLineupEditor,
  args: { channelId: "ch-1" },
  decorators: [withStubbedLineup(), widthFrame(560)],
} satisfies Meta<typeof ChannelLineupEditor>;

type Story = StoryObj<typeof meta>;

// The common case: a populated lineup with drag handles, remove buttons, and the add
// affordance — the initial render the visual gallery snapshots (no play()).
const Populated: Story = {
  args: {
    lineup: [
      { key: "movie:tmdb:949", name: "Heat", year: 1995 },
      { key: "movie:tmdb:9426", name: "Point Break", year: 1991 },
      { key: "movie:tmdb:1892", name: "Return of the Jedi", year: 1983 },
    ],
  },
};

// An empty lineup — the dashed empty state, not an error.
const Empty: Story = { args: { lineup: [] } };

// Ready + pending side by side: play() runs the real add flow (open the palette, search,
// pick the not-in-library candidate) so the resulting Pending badge is the actual
// component behaviour, not a prop invented for the story. "Pending" has no field on
// LineupEntryDTO to seed directly (the backend's read-back never carries an
// availability flag, see the component header) — the moment it becomes visible is
// exactly this add, which is why this story demonstrates it rather than the default one.
const WithPendingAdd: Story = {
  args: Populated.args,
  play: async ({ canvas, userEvent }) => {
    await userEvent.click(canvas.getByRole("button", { name: /add a title/i }));
    await userEvent.type(canvas.getByLabelText("Search"), "breaking");
    await userEvent.click(await canvas.findByText("Breaking Bad"));
    await canvas.findByText("Pending");
  },
};

export default meta;
export { Empty, Populated, WithPendingAdd };
