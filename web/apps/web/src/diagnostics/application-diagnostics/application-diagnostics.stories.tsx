import type { ListDiagnosticEvents200One } from "@loomarr/api";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useState } from "react";
import { ApplicationDiagnostics, DEFAULT_APPLICATION_FILTERS } from "./application-diagnostics";

const observedAt = Date.UTC(2026, 7, 23, 23, 14);
const dropState = { dropped: 2 };
const events: ListDiagnosticEvents200One = {
  items: [
    {
      id: "evt-transition",
      occurredAt: observedAt,
      receivedAt: observedAt + 83,
      level: "error",
      source: "web",
      subsystem: "player",
      event: "player.transition_failed",
      message: "The replacement source did not become playable before the handoff deadline.",
      playbackSessionId: "play_01K3D8J2",
      channelId: "channel-7",
      scheduleBlockId: "block-commercial-42",
      processRunId: "process-ffmpeg-19",
      attributes: { transport: "hls_js", ready_state: 1, drift_ms: 2140 },
    },
    {
      id: "evt-late",
      occurredAt: observedAt - 4_000,
      receivedAt: observedAt - 3_940,
      level: "warn",
      source: "server",
      subsystem: "playout",
      event: "playout.transition_late",
      message: "Schedule authority advanced before the current segment completed.",
      channelId: "channel-7",
      scheduleBlockId: "block-commercial-42",
      attributes: { late_by_ms: 1840 },
    },
    {
      id: "evt-ready",
      occurredAt: observedAt - 9_000,
      receivedAt: observedAt - 8_960,
      level: "info",
      source: "android_tv",
      subsystem: "player",
      event: "player.ready",
      playbackSessionId: "play_01K3D8J2",
      channelId: "channel-7",
      attributes: { transport: "media3" },
    },
  ],
  nextCursor: "older-page",
  ...dropState,
};

const jsonResponse = (body: ListDiagnosticEvents200One) =>
  new Response(JSON.stringify(body), { status: 200, headers: { "content-type": "application/json" } });

type StoryMode = "populated" | "empty" | "loading" | "disconnected";

const DiagnosticsStory = ({ mode: _mode }: { mode: StoryMode }) => {
  const [filters, setFilters] = useState(DEFAULT_APPLICATION_FILTERS);
  return (
    <ApplicationDiagnostics filters={filters} onFiltersChange={setFilters} onOpenProcess={() => undefined} />
  );
};

const withDiagnostics = (Story: typeof DiagnosticsStory, context: { args: { mode?: StoryMode } }) => {
  window.fetch = ((input) => {
    const url = typeof input === "string" ? input : input instanceof URL ? input.href : input.url;
    if (context.args.mode === "loading") return new Promise(() => undefined);
    if (context.args.mode === "disconnected") return Promise.reject(new Error("Loomarr is unreachable"));
    return Promise.resolve(
      url.includes("verbose-capture")
        ? new Response(JSON.stringify({ active: false }), {
            status: 200,
            headers: { "content-type": "application/json" },
          })
        : jsonResponse(context.args.mode === "empty" ? { items: [], dropped: 0 } : events),
    );
  }) as typeof fetch;
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return (
    <QueryClientProvider client={client}>
      <Story mode={context.args.mode ?? "populated"} />
    </QueryClientProvider>
  );
};

const meta = {
  title: "Diagnostics/Logs",
  component: DiagnosticsStory,
  decorators: [
    (Story) => (
      <div style={{ width: "min(1100px, calc(100vw - 32px))" }}>
        <Story />
      </div>
    ),
    withDiagnostics,
  ],
  args: { mode: "populated" },
} satisfies Meta<typeof DiagnosticsStory>;

type Story = StoryObj<typeof meta>;

const Populated: Story = {};
const Empty: Story = { args: { mode: "empty" } };
const Loading: Story = { args: { mode: "loading" } };
const Disconnected: Story = { args: { mode: "disconnected" } };
const AdvancedFilters: Story = {
  play: async ({ canvas, userEvent }) => {
    await canvas.findByText(/replacement source did not become playable/i);
    await userEvent.click(canvas.getByRole("button", { name: "More filters" }));
    await canvas.findByRole("textbox", { name: "Subsystem" });
  },
};
const ProcessDetail: Story = {
  play: async ({ canvas, userEvent }) => {
    const row = await canvas.findByRole("button", { name: /replacement source did not become playable/i });
    await userEvent.click(row);
    await canvas.findAllByRole("button", { name: "Open process output" });
  },
};

export default meta;
export { AdvancedFilters, Disconnected, Empty, Loading, Populated, ProcessDetail };
