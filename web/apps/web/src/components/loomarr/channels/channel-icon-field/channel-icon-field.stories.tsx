import type { IconSuggestionsOutputBody } from "@loomarr/api";
import type { Decorator, Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { widthFrame } from "@/test/story-utils";
import { ChannelIconField } from "./channel-icon-field";

const noop = async () => {};

// ⚠ Generic, with the success call site passing its type argument explicitly (GH #281). The
// parameter stays open because this helper serialises both success bodies AND the RFC 7807
// problem body the non-200 story returns — one DTO could not describe both. An unchecked success
// body is invisible to `tsc`, so a response gaining a required field leaves the stub stale and
// the failure surfaces as a Playwright baseline diff that reads like a rendering regression.
const jsonResponse = <T,>(body: T, status = 200) =>
  new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });

// ChannelIconField owns the live `useChannelIconSuggestions` hook itself (same shape as
// RefinePanel elsewhere in this package) rather than taking suggestions as a prop, so a
// story needs a QueryClient + a stubbed `fetch` to answer the icon-suggestions request
// deterministically — no backend, no new dependency (the same approach refine-panel's
// story uses).
//
// All image URLs here are inline data: URIs, NOT remote tmdb.org links — for BOTH the
// suggestion grid (POSTERS) and the displayed `logo`. Reason: <img src> bypasses the
// stubbed `fetch` and loads over the browser's own network, so a remote URL made these
// stories flaky (the snapshot could fire before the image decoded/painted — worse for the
// grid's `loading="lazy"` posters). A data URI paints synchronously with no network;
// SuggestionsLoaded's play() additionally waits for decode. Three distinct solid-color
// 1×1 PNGs keep the posters visually distinguishable in the grid.
// biome-ignore format: data-URI payloads read better unwrapped
const TNG_POSTER = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNg+M8AAAMBAQDJ/pLvAAAAAElFTkSuQmCC";
// A suggestion's image record. Since V52 phase 7 the backend adopts each TMDB poster and only
// offers a suggestion once its bytes exist, so `image` present is the ORDINARY state here — which
// is why the stories carry one: a grid of `<img>` fallbacks would snapshot the path the picker no
// longer takes.
//
// ⚠ Same-origin assets from `.storybook/story-assets/`, NOT data URIs. A base64 data URI always
// contains a comma — `srcset`'s candidate separator — so it is unloadable there (#210), and remote
// URLs race the snapshot. Each rung is a different colour so the baseline shows which one the
// grid's box selected.
const suggestionImage = (hash: string) => ({
  hash,
  role: "icon",
  width: 500,
  height: 500,
  placeholder: "1QcSHQRnh493V4dIh4eXh1h4kJUI",
  dominantHex: "#2b4a5e",
  animated: false,
  srcSetWebp: "/icon-92.webp 92w, /icon-185.webp 185w, /icon-500.webp 500w",
  // Empty: AVIF is job-produced, so a just-adopted poster has none — the ordinary state.
  srcSetAvif: "",
  src: "/icon-fallback.jpg",
});

const POSTERS = [
  { title: "The Next Generation", url: TNG_POSTER, image: suggestionImage("a".repeat(64)) },
  // biome-ignore format: data-URI payloads read better unwrapped
  { title: "Deep Space Nine", url: "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNgYPj/HwAEAQH/7uJ9WQAAAABJRU5ErkJggg==", image: suggestionImage("b".repeat(64)) },
  // biome-ignore format: data-URI payloads read better unwrapped
  { title: "Voyager", url: "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8z8BQzwAEjP8ZAAoDAv8T7QhZAAAAAElFTkSuQmCC", image: suggestionImage("c".repeat(64)) },
];

const withSuggestions =
  (suggestions: typeof POSTERS = POSTERS, status = 200): Decorator =>
  (Story) => {
    window.fetch = ((url: string) => {
      if (typeof url === "string" && url.includes("/icon-suggestions")) {
        // ⚠ Only the SUCCESS branch is typed. The non-200 branch returns an RFC 7807 problem
        // document, not the success DTO, so annotating it `IconSuggestionsOutputBody` would be
        // wrong rather than safer.
        return status === 200
          ? Promise.resolve(jsonResponse<IconSuggestionsOutputBody>({ suggestions }))
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
  title: "Channels/ChannelIconField",
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
    logo: TNG_POSTER,
  },
  decorators: [withSuggestions()],
};

// ⚠ The service-hosted case (V52 phase 5), and the reason it is a SEPARATE story from WithIcon.
// When `logoImage` is present the preview renders through the <Image> primitive — srcset at the
// icon ladder, a ThumbHash underneath, and the field's own glyph as the designed failure state.
// `WithIcon` above keeps exercising the plain-<img> path, which is what an operator-pasted
// external URL still gets; both paths ship, so both need a baseline.
const WithServiceHostedIcon: Story = {
  args: {
    isAdmin: true,
    logo: "/v1/images/9f2b1c4e8a7d65039f2b1c4e8a7d65039f2b1c4e8a7d65039f2b1c4e8a7d6503/w500.jpg",
    logoImage: {
      hash: "9f2b1c4e8a7d65039f2b1c4e8a7d65039f2b1c4e8a7d65039f2b1c4e8a7d6503",
      role: "icon",
      width: 512,
      height: 512,
      placeholder: "1QcSHQRnh493V4dIh4eXh1h4kJUI",
      dominantHex: "#2b4a5e",
      animated: false,
      // ⚠ Same-origin static assets from `.storybook/story-assets/`, NOT data URIs. A base64 data
      // URI always contains a comma — `srcset`'s candidate separator — so it is unloadable there
      // (#210); remote URLs are banned because they race the snapshot. Each rung is a different
      // colour, so the baseline shows WHICH one a 64px box selected (the 92w rung).
      srcSetWebp: "/icon-92.webp 92w, /icon-185.webp 185w, /icon-500.webp 500w",
      // Empty: AVIF is job-produced, so a freshly-uploaded icon has none — the ordinary state.
      srcSetAvif: "",
      src: "/icon-fallback.jpg",
    },
  },
  decorators: [withSuggestions()],
};

// A viewer (non-admin): the icon shows, but none of the editing affordances render.
const ViewerOnly: Story = {
  args: {
    isAdmin: false,
    logo: TNG_POSTER,
  },
  decorators: [withSuggestions()],
};

// Opening "Change icon" loads the lineup's TMDB posters into the suggestions grid.
const SuggestionsLoaded: Story = {
  args: { isAdmin: true },
  decorators: [withSuggestions()],
  play: async ({ canvas, userEvent, expect, waitFor }) => {
    await userEvent.click(await canvas.findByRole("button", { name: /change icon/i }));
    await canvas.findByRole("button", { name: /use the next generation's poster/i });
    // The poster button existing ≠ its <img> having painted (the imgs are `loading="lazy"`).
    // Wait until every poster image has actually decoded before the snapshot fires, so the
    // grid is never captured half-blank. With the data: URIs above this resolves on the
    // first tick, but the assertion keeps the story deterministic regardless of the source.
    await waitFor(() => {
      const imgs = canvas.getAllByRole("img");
      for (const img of imgs) {
        expect((img as HTMLImageElement).complete).toBe(true);
        expect((img as HTMLImageElement).naturalWidth).toBeGreaterThan(0);
      }
    });
  },
};

// TMDB isn't configured on this install (§icon suggestions return 501) — guidance copy,
// not an error state, since a search-only deployment is a normal shape.
const TmdbNotConfigured: Story = {
  args: { isAdmin: true },
  decorators: [withSuggestions([], 501)],
  play: async ({ canvas, userEvent }) => {
    await userEvent.click(await canvas.findByRole("button", { name: /change icon/i }));
    await canvas.findByText(/set up a tmdb connection/i);
  },
};

export default meta;
export { NoIcon, SuggestionsLoaded, TmdbNotConfigured, ViewerOnly, WithIcon, WithServiceHostedIcon };
