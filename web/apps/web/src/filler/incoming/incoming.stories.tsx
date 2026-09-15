import type { FillerIncomingOutputBody, IncomingStatusDTO } from "@loomarr/api";
import type { Decorator, Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { withRouter } from "@/test/story-utils";
import { Incoming } from "./incoming";

const status = (index: number, statusLabel = "Adding details"): IncomingStatusDTO => ({
  clipHash: `clip-${index}`,
  name: index === 1 ? "Tootsie Pop classic commercial" : `Commercial ${index}`,
  from: index % 2 === 0 ? "Classic TV Commercials" : "Commercials From The Vault",
  durationMs: 25_000 + index * 1_000,
  statusLabel,
  updatedAt: "2026-09-14T20:00:00Z",
  technical: {
    attempts: 1,
    stages: [
      { label: "Checking video", status: "Finished", at: "2026-09-14T19:59:00Z" },
      {
        label: "Adding details",
        status: statusLabel === "Ready" ? "Finished" : "In progress",
        at: "2026-09-14T20:00:00Z",
      },
    ],
  },
});

const empty: FillerIncomingOutputBody = {
  preparing: { rows: [], total: 0 },
  needsHelp: { rows: [], total: 0 },
  recentlyReady: { rows: [], total: 0 },
};

const withIncoming =
  (body: FillerIncomingOutputBody): Decorator =>
  (Story) => {
    window.fetch = (() =>
      Promise.resolve(
        new Response(JSON.stringify(body), {
          status: 200,
          headers: { "content-type": "application/json" },
        }),
      )) as typeof fetch;
    return (
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
        <Story />
      </QueryClientProvider>
    );
  };

const frame: Decorator = (Story) => (
  <div style={{ width: "100%", maxWidth: 960 }}>
    <Story />
  </div>
);

const meta = {
  title: "Filler/Incoming",
  component: Incoming,
  decorators: [frame, withRouter("/filler/incoming")],
} satisfies Meta<typeof Incoming>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Empty: Story = { decorators: [withIncoming(empty)] };

export const PreparingAndReady: Story = {
  decorators: [
    withIncoming({
      preparing: { rows: [status(1), status(2, "Checking video")], total: 2 },
      needsHelp: { rows: [], total: 0 },
      recentlyReady: { rows: [status(3, "Ready")], total: 1 },
    }),
  ],
};

export const NeedsHelp: Story = {
  decorators: [
    withIncoming({
      preparing: { rows: [status(1)], total: 1 },
      needsHelp: {
        rows: [
          {
            id: "split-1",
            kind: "split_boundary",
            clipHash: "reel-1",
            name: "Saturday morning commercial reel",
            question: "Where should this compilation be split?",
            actionLabel: "Review clips",
            actionHref: "/filler/splits/split-1",
            createdAt: "2026-09-14T19:00:00Z",
          },
        ],
        total: 3,
        nextCursor: "more-choices",
      },
      recentlyReady: { rows: [], total: 0 },
    }),
  ],
};

export const TwentyPlusClips: Story = {
  decorators: [
    withIncoming({
      preparing: {
        rows: Array.from({ length: 20 }, (_, index) => status(index + 1)),
        total: 47,
        nextCursor: "page-2",
      },
      needsHelp: { rows: [], total: 0 },
      recentlyReady: { rows: [], total: 0 },
    }),
  ],
};
