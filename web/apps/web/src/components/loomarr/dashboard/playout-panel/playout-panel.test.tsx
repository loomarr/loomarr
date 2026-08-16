import type { ChannelHealth, PlayoutStatus } from "@loomarr/api";
import { render, screen } from "@testing-library/react";
import type { ReactElement } from "react";
import { describe, expect, it } from "vitest";
import { TooltipProvider } from "@/components/ui";
import { PlayoutPanel } from "./playout-panel";

// The channel-id affordance uses the app-wide Tooltip, whose provider is mounted at __root.tsx in
// the running app. In isolation the test supplies it — same as any component under test that uses a
// Radix tooltip.
const renderPanel = (ui: ReactElement) => render(<TooltipProvider>{ui}</TooltipProvider>);

// The merged dashboard playout panel: GPU/LLM-contention header + one row per channel. What is
// pinned here is the merge's promises — the human channel label (not the raw id), the id kept
// behind an affordance, the folded-in throughput/cold-start, and the contention badge.

const channel = (over: Partial<ChannelHealth> = {}): ChannelHealth => ({
  channelId: "ch_9bd1fd7609b5f925",
  number: 3,
  name: "1980s Action Heroes",
  target: "browser",
  viewers: 2,
  bufferedMs: 7_000,
  encoder: "h264_nvenc",
  hardware: true,
  mode: "transcode",
  speed: 3.2,
  health: "ok",
  reason: "Encoding comfortably ahead of realtime.",
  coldStartMs: 1_300,
  ...over,
});

const status = (over: Partial<PlayoutStatus> = {}): PlayoutStatus =>
  ({
    running: true,
    gpu: { name: "NVIDIA GeForce RTX 3080 Ti", vramGiB: 12, contended: false },
    prepared: {
      available: true,
      running: false,
      channels: 0,
      readyChannels: 0,
      scheduledBindings: 0,
      readyBindings: 0,
      missingBindings: 0,
      queuedPublications: 0,
      remainingBytes: 0,
      budgetBytes: 0,
      protectedBytes: 0,
    },
    channels: [channel()],
    ...over,
  }) as PlayoutStatus;

describe("PlayoutPanel", () => {
  it("shows the human channel label, not the raw id", () => {
    renderPanel(<PlayoutPanel status={status()} />);
    expect(screen.getByText("#3 · 1980s Action Heroes")).toBeInTheDocument();
    // The raw id is not shown as plain row text — it lives behind the info affordance.
    expect(screen.queryByText("ch_9bd1fd7609b5f925")).not.toBeInTheDocument();
  });

  it("keeps the raw id reachable behind an info control", () => {
    renderPanel(<PlayoutPanel status={status()} />);
    // The affordance is labelled with the id so it is discoverable + accessible without opening it.
    expect(screen.getByRole("button", { name: /Channel id ch_9bd1fd7609b5f925/ })).toBeInTheDocument();
  });

  it("falls back to the id when the channel has no name", () => {
    renderPanel(<PlayoutPanel status={status({ channels: [channel({ name: "", number: 0 })] })} />);
    expect(screen.getByText("ch_9bd1fd7609b5f925")).toBeInTheDocument();
  });

  it("folds in throughput and cold-start on the row", () => {
    renderPanel(<PlayoutPanel status={status()} />);
    expect(screen.getByText(/2 viewers/)).toBeInTheDocument();
    expect(screen.getByText(/h264_nvenc/)).toBeInTheDocument();
    expect(screen.getByText(/1\.3s to play/)).toBeInTheDocument();
  });

  it("accepts a task-specific title while keeping the dashboard default", () => {
    const { rerender } = renderPanel(<PlayoutPanel status={status()} />);
    expect(screen.getByRole("heading", { name: "Playout" })).toBeInTheDocument();

    rerender(
      <TooltipProvider>
        <PlayoutPanel title="Current playback" status={status()} />
      </TooltipProvider>,
    );
    expect(screen.getByRole("heading", { name: "Current playback" })).toBeInTheDocument();
  });

  it("flags GPU contention with a badge", () => {
    renderPanel(
      <PlayoutPanel
        status={status({
          gpu: { name: "GPU", vramGiB: 12, llmModel: "qwen3:8b", llmVramGiB: 5.6, contended: true },
        })}
      />,
    );
    expect(screen.getByText("LLM sharing VRAM")).toBeInTheDocument();
  });

  it("does not mistake prepared viewers for an idle channel", () => {
    renderPanel(<PlayoutPanel status={status({ channels: [] })} />);
    expect(screen.getByText(/No live fallback encoders/)).toBeInTheDocument();
  });

  it("distinguishes a Tunarr-backed install from unhealthy channels", () => {
    renderPanel(<PlayoutPanel status={status({ running: false, channels: [] })} />);
    expect(screen.getByText(/Tunarr is\./)).toBeInTheDocument();
    expect(screen.getByText("Tunarr")).toBeInTheDocument();
  });

  it("names Loomarr as the owner of internal playback", () => {
    renderPanel(<PlayoutPanel status={status()} />);
    expect(screen.getByText("Loomarr")).toBeInTheDocument();
  });

  it("distinguishes a planner that has not run from an empty ready window", () => {
    renderPanel(<PlayoutPanel status={status({ channels: [] })} />);
    expect(screen.getByText(/Readiness pass hasn’t run yet/)).toBeInTheDocument();
  });

  it("shows prepared coverage, warming backlog, and cache pressure together", () => {
    renderPanel(
      <PlayoutPanel
        status={status({
          prepared: {
            available: true,
            running: true,
            lastRunAt: "2026-08-14T12:00:00Z",
            channels: 100,
            readyChannels: 84,
            scheduledBindings: 300,
            readyBindings: 260,
            missingBindings: 40,
            queuedPublications: 16,
            remainingBytes: 700 * 1024 ** 3,
            budgetBytes: 1_000 * 1024 ** 3,
            protectedBytes: 600 * 1024 ** 3,
          },
        })}
      />,
    );
    expect(screen.getByText("84 of 100 channels ready")).toBeInTheDocument();
    expect(screen.getByText(/40 scheduled bindings unprepared/)).toBeInTheDocument();
    expect(screen.getByText(/16 publications warming/)).toBeInTheDocument();
    expect(screen.getByText(/700\.0 GB of 1000\.0 GB used/)).toBeInTheDocument();
    expect(screen.getByText(/600\.0 GB protected/)).toBeInTheDocument();
  });

  it("treats a completed empty window as ready rather than warming", () => {
    renderPanel(
      <PlayoutPanel
        status={status({
          channels: [],
          prepared: {
            ...status().prepared,
            lastRunAt: "2026-08-14T12:00:00Z",
          },
        })}
      />,
    );
    expect(screen.getByText("No scheduled channels in this window")).toBeInTheDocument();
    expect(screen.getByText("Ready")).toBeInTheDocument();
  });
});
