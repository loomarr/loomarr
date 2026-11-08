import type { TimelineOutputBody } from "@loomarr/api";
import type { Decorator, Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { widthFrame } from "@/test/story-utils";
import { ChannelHero } from "./channel-hero";

const jsonResponse = (body: unknown) =>
  new Response(JSON.stringify(body), { status: 200, headers: { "content-type": "application/json" } });

// ChannelHero owns the useChannelTimeline hook, so a story stubs `fetch` to answer
// GET /v1/channels/{id}/timeline with a fixed body, as ChannelUpcoming's stories do. The body is
// typed so a response change fails typecheck. No remote images in stories: with no picture to
// load, the hero shows its generated channel art, which is the state worth a baseline.
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

const meta = {
  title: "Channels/ChannelHero",
  component: ChannelHero,
  args: { channelId: "ch-1", channel: { name: "Saturday Cartoons", number: 7 } },
  decorators: [widthFrame(640)],
} satisfies Meta<typeof ChannelHero>;

export default meta;
type Story = StoryObj<typeof meta>;

// A programme with no backdrop or still: generated art in the channel's own colour.
export const GeneratedArt: Story = {
  decorators: [
    withStubbedTimeline({
      serverNowMs: 0,
      airings: [
        {
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

// An empty schedule still opens on art rather than a blank box.
export const EmptySchedule: Story = {
  decorators: [withStubbedTimeline({ serverNowMs: 0, airings: [] })],
};
