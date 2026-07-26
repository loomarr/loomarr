import type { Decorator, Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { widthFrame } from "@/test/story-utils";
import { ChannelUpcoming } from "./channel-upcoming";

const jsonResponse = (body: unknown) =>
  new Response(JSON.stringify(body), { status: 200, headers: { "content-type": "application/json" } });

// ChannelUpcoming owns the useChannelUpcoming hook, so a story stubs `fetch` to answer
// GET /v1/channels/{id}/upcoming with a fixed body (no backend, no new dependency — the same
// approach ChannelCyclePreview's story uses). Airtimes are fixed epoch-ms so the render is
// deterministic; the "Now" highlight is driven by the `live` prop + start ≤ Date.now(), so a
// live story seeds the first entry's start in the past.
const withStubbedUpcoming =
  (upcoming: unknown): Decorator =>
  (Story) => {
    window.fetch = (() => Promise.resolve(jsonResponse({ upcoming }))) as typeof fetch;
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    return (
      <QueryClientProvider client={client}>
        <Story />
      </QueryClientProvider>
    );
  };

const meta = {
  title: "Channels/ChannelUpcoming",
  component: ChannelUpcoming,
  args: { channelId: "ch-1" },
  decorators: [widthFrame(420)],
} satisfies Meta<typeof ChannelUpcoming>;

export default meta;
type Story = StoryObj<typeof meta>;

// Fixed airtimes (a Saturday-morning block) so the snapshot is stable regardless of the wall
// clock — formatEpgTime renders these in the viewer's locale, pinned by the visual harness.
const NINE_AM = Date.UTC(2026, 6, 25, 9, 0, 0);
const at = (mins: number) => NINE_AM + mins * 60_000;

// A live channel: the first entry is airing now (highlighted), the rest are upcoming.
export const Live: Story = {
  args: { channelId: "ch-1", live: true },
  decorators: [
    withStubbedUpcoming([
      // start far in the past so `start ≤ Date.now()` → the "Now" highlight fires.
      { title: "Grandad", startMs: 0, stopMs: at(30), gap: false },
      { title: "Chuckie's a Lefty", startMs: at(30), stopMs: at(60), gap: false },
      { title: "The Rowdyruff Boys", startMs: at(60), stopMs: at(90), gap: false },
      { title: "The Sleepover", startMs: at(90), stopMs: at(120), gap: false },
    ]),
  ],
};

// Not live (building/paused): entries show their airtimes, none highlighted as "now".
export const Scheduled: Story = {
  args: { channelId: "ch-1", live: false },
  decorators: [
    withStubbedUpcoming([
      { title: "Grandad", startMs: at(0), stopMs: at(30), gap: false },
      { title: "Chuckie's a Lefty", startMs: at(30), stopMs: at(60), gap: false },
      { title: "The Rowdyruff Boys", startMs: at(60), stopMs: at(90), gap: false },
    ]),
  ],
};

// Nothing scheduled — the empty state (live vs off changes the copy).
export const Empty: Story = {
  args: { channelId: "ch-1", live: true },
  decorators: [withStubbedUpcoming([])],
};
