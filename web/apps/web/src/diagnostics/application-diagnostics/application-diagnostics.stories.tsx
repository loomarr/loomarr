import type { ListDiagnosticEvents200One } from "@loomarr/api";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useState } from "react";
import { widthFrame } from "@/test/story-utils";
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

const DiagnosticsStory = () => {
  const [filters, setFilters] = useState(DEFAULT_APPLICATION_FILTERS);
  return (
    <ApplicationDiagnostics filters={filters} onFiltersChange={setFilters} onOpenProcess={() => undefined} />
  );
};

const withDiagnostics = (Story: typeof DiagnosticsStory) => {
  window.fetch = (() => Promise.resolve(jsonResponse(events))) as typeof fetch;
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return (
    <QueryClientProvider client={client}>
      <Story />
    </QueryClientProvider>
  );
};

const meta = {
  title: "Diagnostics/Application",
  component: DiagnosticsStory,
  decorators: [widthFrame(1100), withDiagnostics],
} satisfies Meta<typeof DiagnosticsStory>;

type Story = StoryObj<typeof meta>;

const Timeline: Story = {};
const ExpandedFailure: Story = {
  play: async ({ canvas, userEvent }) => {
    await userEvent.click(await canvas.findByRole("button", { name: /details/i }));
    await canvas.findByText("Structured attributes");
  },
};

export default meta;
export { ExpandedFailure, Timeline };
