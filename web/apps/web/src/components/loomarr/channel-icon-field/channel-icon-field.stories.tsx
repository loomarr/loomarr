import type { Decorator, Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { widthFrame } from "@/test/story-utils";
import { ChannelIconField } from "./channel-icon-field";

const noop = async () => {};

const jsonResponse = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });

// ChannelIconField owns the live `useChannelIconSuggestions` hook itself (same shape as
// RefinePanel elsewhere in this package) rather than taking suggestions as a prop, so a
// story needs a QueryClient + a stubbed `fetch` to answer the icon-suggestions request
// deterministically — no backend, no new dependency (the same approach refine-panel's
// story uses).
const POSTERS = [
  { title: "The Next Generation", url: "https://image.tmdb.org/t/p/w342/tng.jpg" },
  { title: "Deep Space Nine", url: "https://image.tmdb.org/t/p/w342/ds9.jpg" },
  { title: "Voyager", url: "https://image.tmdb.org/t/p/w342/voy.jpg" },
];

const withSuggestions =
  (suggestions: typeof POSTERS = POSTERS, status = 200): Decorator =>
  (Story) => {
    window.fetch = ((url: string) => {
      if (typeof url === "string" && url.includes("/icon-suggestions")) {
        return status === 200
          ? Promise.resolve(jsonResponse({ suggestions }))
          : Promise.resolve(jsonResponse({ title: "TMDB isn't configured" }, status));
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

// The channel-detail "info" section's icon picker (see the component doc comment for the
// three ways in). Stories exercise the read side (preview + suggestions); the upload and
// PATCH paths are covered by the co-located unit test and the live app, since they need a
// real multipart request / mutation, not a Storybook concern.
const meta = {
  title: "Loomarr/ChannelIconField",
  component: ChannelIconField,
  args: {
    channelId: "ch-1",
    onSetLogo: noop,
  },
  decorators: [widthFrame(480)],
} satisfies Meta<typeof ChannelIconField>;

type Story = StoryObj<typeof meta>;

// Admin, no icon set yet — the muted placeholder, "Change icon" collapsed.
const NoIcon: Story = {
  args: { isAdmin: true },
  decorators: [withSuggestions()],
};

// Admin, an icon already set — the 64px preview + Clear button appear.
const WithIcon: Story = {
  args: {
    isAdmin: true,
    logo: "https://image.tmdb.org/t/p/w342/tng.jpg",
  },
  decorators: [withSuggestions()],
};

// A viewer (non-admin): the icon shows, but none of the editing affordances render.
const ViewerOnly: Story = {
  args: {
    isAdmin: false,
    logo: "https://image.tmdb.org/t/p/w342/tng.jpg",
  },
  decorators: [withSuggestions()],
};

// Opening "Change icon" loads the lineup's TMDB posters into the suggestions grid.
const SuggestionsLoaded: Story = {
  args: { isAdmin: true },
  decorators: [withSuggestions()],
  play: async ({ canvas, userEvent }) => {
    await userEvent.click(canvas.getByRole("button", { name: /change icon/i }));
    await canvas.findByRole("button", { name: /use the next generation's poster/i });
  },
};

// TMDB isn't configured on this install (§icon suggestions return 501) — guidance copy,
// not an error state, since a search-only deployment is a normal shape.
const TmdbNotConfigured: Story = {
  args: { isAdmin: true },
  decorators: [withSuggestions([], 501)],
  play: async ({ canvas, userEvent }) => {
    await userEvent.click(canvas.getByRole("button", { name: /change icon/i }));
    await canvas.findByText(/set up a tmdb connection/i);
  },
};

export default meta;
export { NoIcon, SuggestionsLoaded, TmdbNotConfigured, ViewerOnly, WithIcon };
