import type { Decorator, Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { LoomarrEventsProvider } from "@/events";
import { widthFrame } from "@/test/story-utils";
import { RefinePanel } from "./refine-panel";

const jsonResponse = (body: unknown) =>
  new Response(JSON.stringify(body), { status: 200, headers: { "content-type": "application/json" } });

// An EventSource stand-in that records the latest instance so a play() can push a frame.
// The failed-run story needs a real `suggestion` frame to reach the panel; Storybook (like
// jsdom) has no EventSource of its own.
class StoryEventSource {
  static last: StoryEventSource | undefined;
  private handlers = new Map<string, (e: MessageEvent) => void>();
  constructor() {
    StoryEventSource.last = this;
  }
  addEventListener(type: string, cb: (e: MessageEvent) => void) {
    this.handlers.set(type, cb);
  }
  close() {}
  emit(type: string, data: unknown) {
    this.handlers.get(type)?.({ data: JSON.stringify(data) } as MessageEvent);
  }
}

// RefinePanel owns live generated-API hooks (useChannelRefine, useApproveProposal)
// rather than taking injectable state — same shape as ApprovalQueue elsewhere in this
// package, which has no story for the same reason. Unlike that component, this one was
// asked for a story to demonstrate the landed diff, so this decorator stubs `fetch`
// deterministically (no backend, no new dependency) the same way the co-located test
// does: dispatch by method/path, matching the real refine → poll → approve endpoints.
const withStubbedRefine = (): Decorator => (Story) => {
  window.fetch = ((url: string, init?: RequestInit) => {
    if (typeof url === "string" && url.includes("/refine")) {
      return Promise.resolve(jsonResponse({ jobId: "job-1" }));
    }
    if (init?.method === "POST" && typeof url === "string" && url.includes("/approve")) {
      return Promise.resolve(jsonResponse({ channelId: "ch-1", enqueued: 0 }));
    }
    return Promise.resolve(
      jsonResponse({
        proposals: [
          {
            id: "p1",
            jobId: "job-1",
            status: "submitted",
            proposal: {
              intent: { description: "add more Schwarzenegger" },
              lineup: [
                { name: "Heat", year: 1995, mediaType: "movie", tmdbId: 949, inLibrary: true },
                { name: "Predator", year: 1987, mediaType: "movie", tmdbId: 106, inLibrary: true },
              ],
              acquisitions: [],
              alternates: [],
              scores: { themeFit: 0.9, availabilityRatio: 1, eraBalance: 0.7, overall: 0.85 },
            },
          },
        ],
      }),
    );
  }) as typeof fetch;

  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return (
    <QueryClientProvider client={client}>
      <Story />
    </QueryClientProvider>
  );
};

// Same fetch stub, but the run never lands a proposal (a failed run produces none), and
// the tree is wrapped in the events provider with a dispatchable EventSource so play()
// can fire the `failed` frame that drives the panel's inline error.
const withFailingRefine = (): Decorator => (Story) => {
  window.fetch = ((url: string) => {
    if (typeof url === "string" && url.includes("/refine")) {
      return Promise.resolve(jsonResponse({ jobId: "job-1" }));
    }
    return Promise.resolve(jsonResponse({ proposals: [] }));
  }) as typeof fetch;
  (window as unknown as { EventSource: unknown }).EventSource = StoryEventSource;

  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return (
    <QueryClientProvider client={client}>
      <LoomarrEventsProvider>
        <Story />
      </LoomarrEventsProvider>
    </QueryClientProvider>
  );
};

// The "Refine with AI" entry on a channel's detail page (§7 refine).
const meta = {
  title: "Loomarr/RefinePanel",
  component: RefinePanel,
  args: {
    channelId: "ch-1",
    channelName: "90s Action",
    current: [
      { name: "Heat", year: 1995, key: "movie:tmdb:949" },
      { name: "Point Break", year: 1991, key: "movie:tmdb:9426" },
    ],
  },
  // widthFrame only (visual). Each story below brings its OWN fetch/EventSource stub so
  // the happy-path and failure decorators never both patch window.fetch on one render.
  decorators: [widthFrame(560)],
} satisfies Meta<typeof RefinePanel>;

type Story = StoryObj<typeof meta>;

// Collapsed — the default state on a channel page.
const Idle: Story = { decorators: [withStubbedRefine()] };

// Expanded, mid-run and landed: play() opens the panel, submits a change, and waits for
// the diff so the story demonstrates the actual review a reviewer would see.
const Landed: Story = {
  decorators: [withStubbedRefine()],
  play: async ({ canvas, userEvent }) => {
    await userEvent.click(canvas.getByRole("button", { name: /refine with ai/i }));
    await userEvent.type(canvas.getByLabelText("What to change"), "add more Schwarzenegger");
    await userEvent.click(canvas.getByRole("button", { name: /^refine$/i }));
    await canvas.findByRole("button", { name: /apply changes/i });
  },
};

// Generation failed: play() submits a change, then fires a `failed` SSE frame so the panel
// shows its recoverable inline error with the typed change preserved — the "fix it in
// place" state, not a dead end.
const GenerationFailed: Story = {
  decorators: [withFailingRefine(), widthFrame(560)],
  play: async ({ canvas, userEvent }) => {
    await userEvent.click(canvas.getByRole("button", { name: /refine with ai/i }));
    await userEvent.type(canvas.getByLabelText("What to change"), "add more Schwarzenegger");
    await userEvent.click(canvas.getByRole("button", { name: /^refine$/i }));
    StoryEventSource.last?.emit("suggestion", { jobId: "job-1", phase: "failed" });
    await canvas.findByText(/couldn't complete/i);
  },
};

export default meta;
export { GenerationFailed, Idle, Landed };
