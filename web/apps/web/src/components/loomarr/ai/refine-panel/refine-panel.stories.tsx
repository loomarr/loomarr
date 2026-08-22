import type { ApproveOutputBody, ProposalJourneyDTO, RefineChannelOutputBody } from "@loomarr/api";
import type { Decorator, Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { LoomarrEventsProvider } from "@/events";
import { widthFrame } from "@/test/story-utils";
import { RefinePanel } from "./refine-panel";

// ⚠ Generic, with each dispatched endpoint's body passing its type argument explicitly (GH #281).
// The parameter stays open because this helper only serialises — it is the call sites that must
// be checked against a DTO. An unchecked body is invisible to `tsc`, so a response gaining a
// required field leaves the stub stale and the failure surfaces as a Playwright baseline diff
// that reads like a rendering regression.
const jsonResponse = <T,>(body: T) =>
  new Response(JSON.stringify(body), { status: 200, headers: { "content-type": "application/json" } });

class StoryEventSource {
  static last: StoryEventSource | undefined;
  private handlers = new Map<string, (event: MessageEvent) => void>();
  constructor() {
    StoryEventSource.last = this;
  }
  addEventListener(type: string, callback: (event: MessageEvent) => void) {
    this.handlers.set(type, callback);
  }
  close() {}
  emit(type: string, data: unknown) {
    this.handlers.get(type)?.({ data: JSON.stringify(data) } as MessageEvent);
  }
}

const proposal = {
  intent: { description: "add more Schwarzenegger" },
  lineup: [
    { name: "Heat", year: 1995, mediaType: "movie" as const, tmdbId: 949, inLibrary: true },
    { name: "Predator", year: 1987, mediaType: "movie" as const, tmdbId: 106, inLibrary: true },
  ],
  acquisitions: [],
  alternates: [],
  scores: { themeFit: 0.9, availabilityRatio: 1, eraBalance: 0.7, overall: 0.85 },
};

const landedJourney: ProposalJourneyDTO = {
  version: 1,
  jobId: "job-1",
  milestone: "awaiting_approval",
  intent: proposal.intent,
  attempts: [],
  proposal: { id: "p1", status: "submitted", proposal },
  actions: ["review"],
  createdAt: "2026-08-22T10:00:00Z",
  updatedAt: "2026-08-22T10:00:00Z",
};

// RefinePanel owns live generated-API hooks (useChannelRefine, useApproveProposal)
// rather than taking injectable state — same shape as ApprovalQueue elsewhere in this
// package, which has no story for the same reason. Unlike that component, this one was
// asked for a story to demonstrate the landed diff, so this decorator stubs `fetch`
// deterministically (no backend, no new dependency) the same way the co-located test
// does: dispatch by method/path, matching the real refine → Journey → approve endpoints.
const withStubbedRefine =
  (journey: ProposalJourneyDTO = landedJourney): Decorator =>
  (Story) => {
    window.sessionStorage.removeItem("loomarr.activeChannelRefine");
    window.fetch = ((url: string, init?: RequestInit) => {
      if (typeof url === "string" && url.includes("/refine")) {
        return Promise.resolve(jsonResponse<RefineChannelOutputBody>({ jobId: "job-1" }));
      }
      if (init?.method === "POST" && typeof url === "string" && url.includes("/approve")) {
        // ⚠ `status` was MISSING here until the body was typed — a live example of exactly what
        // #281 is about. The stub predates the field, `tsc` had nothing to compare it against, and
        // the story kept passing because this particular response is never read by an assertion.
        // It would have gone on lying until something did read it.
        return Promise.resolve(
          jsonResponse<ApproveOutputBody>({ channelId: "ch-1", enqueued: 0, status: "approved" }),
        );
      }
      return Promise.resolve(jsonResponse<ProposalJourneyDTO>(journey));
    }) as typeof fetch;

    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    return (
      <QueryClientProvider client={client}>
        <Story />
      </QueryClientProvider>
    );
  };

// The terminal SSE frame can arrive before the authoritative Journey refetch. This fixture
// deliberately leaves that read pending so the panel proves its immediate, safe fallback;
// unit tests separately pin restoration from the persisted failed Journey.
const withFailingRefine = (): Decorator => (Story) => {
  window.sessionStorage.removeItem("loomarr.activeChannelRefine");
  window.fetch = ((url: string) => {
    if (typeof url === "string" && url.includes("/refine")) {
      return Promise.resolve(jsonResponse<RefineChannelOutputBody>({ jobId: "job-1" }));
    }
    return new Promise<Response>(() => {});
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
  title: "AI/RefinePanel",
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
    await userEvent.click(await canvas.findByRole("button", { name: /refine with ai/i }));
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
    await userEvent.click(await canvas.findByRole("button", { name: /refine with ai/i }));
    await userEvent.type(canvas.getByLabelText("What to change"), "add more Schwarzenegger");
    await userEvent.click(canvas.getByRole("button", { name: /^refine$/i }));
    StoryEventSource.last?.emit("suggestion", { jobId: "job-1", phase: "failed" });
    await canvas.findByText(/couldn't complete/i);
  },
};

export default meta;
export { GenerationFailed, Idle, Landed };
