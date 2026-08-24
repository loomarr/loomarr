import type { Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useState } from "react";
import { widthFrame } from "@/test/story-utils";
import type { DiagnosticsSearch } from "../diagnostics-page";
import { PlayoutDiagnostics } from "./playout-diagnostics";

const startedAt = Date.UTC(2026, 7, 24, 3, 42);
const run = {
  id: "process-ffmpeg-19",
  purpose: "channel-segment",
  status: "running",
  startedAt,
  updatedAt: startedAt + 12_000,
  channelId: "channel-7",
  scheduleBlockId: "block-commercial-42",
  executable: "ffmpeg",
  executableVersion: "8.0",
  target: "commercial-02.ts",
  outputBytes: 8_240,
  discardedLines: 14,
};
const initial: DiagnosticsSearch = {
  view: "process",
  range: "1h",
  level: "all",
  source: "all",
  subsystem: "",
  text: "",
  processRange: "1h",
  processStatus: "all",
  processPurpose: "",
  processChannelId: "",
  processJobId: "",
};

const StoryHarness = () => {
  const [filters, setFilters] = useState(initial);
  return <PlayoutDiagnostics filters={filters} onFiltersChange={setFilters} />;
};

const withDiagnostics = (Story: typeof StoryHarness) => {
  window.fetch = ((input) => {
    const url = typeof input === "string" ? input : input instanceof URL ? input.href : input.url;
    if (url.endsWith("/output"))
      return Promise.resolve(
        new Response(
          "[2026-08-24T03:42:10Z] frame=284 fps=29.97 speed=1.01x\n[2026-08-24T03:42:11Z] muxer: continuing segment output",
          {
            status: 200,
            headers: {
              "content-type": "text/plain",
              "X-Diagnostic-Truncated": "true",
              "X-Diagnostic-Discarded-Lines": "14",
            },
          },
        ),
      );
    if (url.includes("process-ffmpeg-19"))
      return Promise.resolve(
        new Response(
          JSON.stringify({
            run,
            progress: [{ frame: 284, occurredAt: startedAt + 12_000, outTimeMs: 9_480, speed: 1.01 }],
            progressTruncated: true,
          }),
          { status: 200, headers: { "content-type": "application/json" } },
        ),
      );
    return Promise.resolve(
      new Response(
        JSON.stringify({
          items: [
            run,
            {
              ...run,
              id: "process-ffmpeg-18",
              status: "failed",
              purpose: "hls-segment",
              startedAt: startedAt - 40_000,
              updatedAt: startedAt - 30_000,
              target: "segment-991.ts",
            },
          ],
        }),
        { status: 200, headers: { "content-type": "application/json" } },
      ),
    );
  }) as typeof fetch;
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return (
    <QueryClientProvider client={client}>
      <Story />
    </QueryClientProvider>
  );
};

const meta = {
  title: "Diagnostics/ProcessRuns",
  component: StoryHarness,
  decorators: [widthFrame(1200), withDiagnostics],
} satisfies Meta<typeof StoryHarness>;
type Story = StoryObj<typeof meta>;
const ActiveRun: Story = {};

export default meta;
export { ActiveRun };
