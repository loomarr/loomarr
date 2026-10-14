import type { PreviewProgrammingChangesOutputBody, ProgrammingChangeDTO } from "@loomarr/api";
import type { Decorator, Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { widthFrame } from "@/test/story-utils";
import { ChannelProgrammingChanges } from "./channel-programming-changes";

// Stubs `fetch` deterministically: POST /programming/changes answers with a fixed body (typed,
// so a contract change breaks the story at tsc rather than letting it drift, see #281), never
// answers, or fails. Each play() waits for the terminal state, because the request only fires
// after the draft debounce and a snapshot taken before then shows "Loading".
const withStub =
  (respond: () => Promise<Response>): Decorator =>
  (Story) => {
    window.fetch = respond as typeof fetch;
    const client = new QueryClient({ defaultOptions: { mutations: { retry: false } } });
    return (
      <QueryClientProvider client={client}>
        <Story />
      </QueryClientProvider>
    );
  };

const answer = (body: PreviewProgrammingChangesOutputBody) =>
  withStub(() =>
    Promise.resolve(
      new Response(JSON.stringify(body), { status: 200, headers: { "content-type": "application/json" } }),
    ),
  );

// Sunday 2026-10-04 18:00 UTC, so the rows read the same in every renderer.
const SIX_PM = Date.UTC(2026, 9, 4, 18);
const HALF_HOUR = 30 * 60 * 1000;
const base = { id: "", label: "Base policy", priority: 0, matched: false };
const marathon = { id: "r3", label: "Sundays 6 PM–8 PM · marathon", priority: 30, matched: true };

const slot = (i: number, before: string, after: string, series?: string): ProgrammingChangeDTO => {
  const startMs = SIX_PM + i * HALF_HOUR;
  const stopMs = startMs + HALF_HOUR;
  return {
    startMs,
    endMs: stopMs,
    before: { kind: "program", title: before, key: `movie:tmdb:${i}`, startMs, stopMs, rule: base },
    after: {
      kind: "program",
      title: after,
      series,
      key: `series:tmdb:${i}`,
      startMs,
      stopMs,
      rule: marathon,
    },
  };
};

const changes: ProgrammingChangeDTO[] = [
  slot(0, "Late Harbor", "Pilot", "The Quiet Coast"),
  slot(1, "Field Notes", "Low Tide", "The Quiet Coast"),
  {
    ...slot(2, "Field Notes", ""),
    after: { ...slot(2, "", "").after!, kind: "break", title: undefined, key: undefined },
  },
  { ...slot(3, "Late Harbor", ""), after: undefined },
];

const span = { fromMs: SIX_PM, toMs: SIX_PM + 24 * 60 * 60 * 1000, compared: 42 };
const draftPolicy = { rules: [{ id: "r3", label: "Sundays 6 PM–8 PM · marathon", priority: 30 }] };

const meta = {
  title: "Channels/ChannelProgrammingChanges",
  component: ChannelProgrammingChanges,
  args: { channelId: "ch-1", draftPolicy, timeZone: "UTC" },
  decorators: [widthFrame(640)],
} satisfies Meta<typeof ChannelProgrammingChanges>;

type Story = StoryObj<typeof meta>;

// The rules draft changes four slots: episodes named by series, a break, and a slot where the
// draft airs nothing.
const WithChanges: Story = {
  decorators: [answer({ ...span, count: changes.length, truncated: false, changes })],
  play: async ({ canvas }) => {
    await canvas.findByText(/4 changes until/);
  },
};

// The draft changes more slots than the response carries: the count says so.
const Truncated: Story = {
  decorators: [
    answer({
      ...span,
      count: 150,
      truncated: true,
      changes: Array.from({ length: 100 }, (_, i) => slot(i, `Saved ${i + 1}`, `Draft ${i + 1}`)),
    }),
  ],
  play: async ({ canvas }) => {
    await canvas.findByText(/Showing 100 of 150 changes/);
  },
};

// The draft airs exactly what is saved.
const NoChanges: Story = {
  decorators: [answer({ ...span, count: 0, truncated: false, changes: [] })],
  play: async ({ canvas }) => {
    await canvas.findByText("No upcoming slots change");
  },
};

// Nothing airs in the span, so there is nothing to compare.
const NothingScheduled: Story = {
  decorators: [answer({ ...span, compared: 0, count: 0, truncated: false, changes: [] })],
  play: async ({ canvas }) => {
    await canvas.findByText("Nothing scheduled");
  },
};

// The comparison is still running.
const Loading: Story = {
  decorators: [withStub(() => new Promise<Response>(() => {}))],
};

const Failed: Story = {
  decorators: [
    withStub(() =>
      Promise.resolve(
        new Response(JSON.stringify({ title: "Internal Server Error", status: 500 }), {
          status: 500,
          headers: { "content-type": "application/problem+json" },
        }),
      ),
    ),
  ],
  play: async ({ canvas }) => {
    await canvas.findByRole("button", { name: /try again/i });
  },
};

// No unsaved rules, so nothing is sent.
const NothingUnsaved: Story = {
  args: { draftPolicy: undefined },
  decorators: [withStub(() => new Promise<Response>(() => {}))],
};

export default meta;
export { Failed, Loading, NoChanges, NothingScheduled, NothingUnsaved, Truncated, WithChanges };
