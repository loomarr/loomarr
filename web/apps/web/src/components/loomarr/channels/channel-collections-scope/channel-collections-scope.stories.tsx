import type { Decorator, Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { TooltipProvider } from "@/components/ui";
import { widthFrame } from "@/test/story-utils";
import { ChannelCollectionsScope } from "./channel-collections-scope";

const noop = () => {};

const jsonResponse = (status: number, body: unknown) =>
  new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });

// Like ChannelSeriesScope's story, this component owns a live generated-API hook rather than
// taking injectable state, so `fetch` is stubbed deterministically — no backend, no new
// dependency (§14). Each story picks the response it needs, because the interesting states
// here ARE the responses (populated / none made yet / no library at all).
const withStubbedCollections =
  (status: number, body: unknown): Decorator =>
  (Story) => {
    window.fetch = ((url: string) => {
      if (typeof url === "string" && url.includes("/v1/library/collections")) {
        return Promise.resolve(jsonResponse(status, body));
      }
      return Promise.resolve(jsonResponse(200, {}));
    }) as typeof fetch;

    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    return (
      <QueryClientProvider client={client}>
        <TooltipProvider>
          <Story />
        </TooltipProvider>
      </QueryClientProvider>
    );
  };

const COLLECTIONS = {
  collections: [
    { id: "bs-1", name: "Halloween", childCount: 12 },
    { id: "bs-2", name: "Criterion", childCount: 88 },
    { id: "bs-3", name: "Christmas" },
  ],
};

// `policy.scope.collections` — the "only what I shelved" narrowing. A checkbox list rather than
// a search box (unlike ChannelSeriesScope), because collections are a small closed set the
// operator curates by hand.
const meta = {
  title: "Channels/ChannelCollectionsScope",
  component: ChannelCollectionsScope,
  args: { onChange: noop },
  decorators: [withStubbedCollections(200, COLLECTIONS), widthFrame(480)],
} satisfies Meta<typeof ChannelCollectionsScope>;

type Story = StoryObj<typeof meta>;

// Nothing ticked = no restriction.
const Empty: Story = { args: { policy: {} } };

// Two of three shelved. The count beside a name is the collection's own size.
const Picked: Story = {
  args: { policy: { scope: { collections: ["bs-1", "bs-3"] } } },
};

// A real answer, not a failure: a library is connected but the operator has made no
// collections. The empty state has to say so — an empty box reads as a broken control.
const NoCollectionsYet: Story = {
  args: { policy: {} },
  decorators: [withStubbedCollections(200, { collections: [] })],
};

export default meta;
export { Empty, NoCollectionsYet, Picked };
