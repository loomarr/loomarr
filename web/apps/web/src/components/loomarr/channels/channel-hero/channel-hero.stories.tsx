import type { TimelineOutputBody } from "@loomarr/api";
import type { ImageDTO } from "@loomarr/api/models/imageDTO";
import type { Decorator, Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { widthFrame } from "@/test/story-utils";
import { ChannelHero } from "./channel-hero";

const jsonResponse = (body: unknown) =>
  new Response(JSON.stringify(body), { status: 200, headers: { "content-type": "application/json" } });

// ChannelHero owns the useChannelTimeline hook, so a story stubs `fetch` to answer
// GET /v1/channels/{id}/timeline with a fixed body, as ChannelUpcoming's stories do. The body is
// typed so a response change fails typecheck. The frame is wider than the hero's 48rem cap, so a
// baseline shows the 16:9 box stopping at 768px.
//
// A channel with no picture draws no hero at all, so it has no story; the component's test holds
// that state.
const withStubbedTimeline =
  (body: Omit<TimelineOutputBody, "$schema">): Decorator =>
  (Story) => {
    window.fetch = (() => Promise.resolve(jsonResponse(body))) as typeof fetch;
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    return (
      <QueryClientProvider client={client}>
        <Story />
      </QueryClientProvider>
    );
  };

// Same-origin files from `.storybook/story-assets/`: no remote image races the snapshot.
const backdrop: ImageDTO = {
  animated: false,
  dominantHex: "#7a5230",
  hash: "9f2b1c4e8a7d65039f2b1c4e8a7d65039f2b1c4e8a7d65039f2b1c4e8a7d6503",
  height: 720,
  placeholder: "",
  role: "backdrop",
  src: "/poster-fallback.jpg",
  srcSetAvif: "",
  srcSetWebp: "/poster-154.webp 154w, /poster-342.webp 342w, /poster-500.webp 500w",
  width: 1280,
};

const meta = {
  title: "Channels/ChannelHero",
  component: ChannelHero,
  args: { channelId: "ch-1", channel: { name: "Saturday Cartoons", number: 7 } },
  decorators: [widthFrame(1024)],
} satisfies Meta<typeof ChannelHero>;

export default meta;
type Story = StoryObj<typeof meta>;

// The backdrop of what is on now.
export const Picture: Story = {
  decorators: [
    withStubbedTimeline({
      serverNowMs: 0,
      airings: [
        {
          backdropImage: backdrop,
          kind: "program",
          scheduleBlockId: "b1",
          title: "Grandad",
          startMs: 0,
          stopMs: Number.MAX_SAFE_INTEGER,
        },
      ],
    }),
  ],
};

// On a phone the hero is the full column width, still 16:9.
export const Phone: Story = {
  decorators: [widthFrame(390), ...(Picture.decorators as Decorator[])],
};
