import type { ChannelHealth, PlayoutDoctor } from "@loomarr/api";
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { PlayoutHealthPanel } from "./playout-health-panel";

const doctor = (over: Partial<PlayoutDoctor> = {}): PlayoutDoctor =>
  ({
    running: true,
    gpu: { vramGiB: 12, contended: false },
    channels: [],
    ...over,
  }) as PlayoutDoctor;

const channel = (over: Partial<ChannelHealth> = {}): ChannelHealth => ({
  channelId: "ch-42",
  target: "browser",
  encoder: "h264_vulkan",
  hardware: true,
  mode: "transcode",
  speed: 3.2,
  health: "ok",
  reason: "Encoding comfortably ahead of realtime.",
  ...over,
});

describe("PlayoutHealthPanel", () => {
  it("shows a row per channel with its reason", () => {
    render(<PlayoutHealthPanel doctor={doctor({ channels: [channel(), channel({ channelId: "ch-7" })] })} />);
    expect(screen.getByText("ch-42")).toBeInTheDocument();
    expect(screen.getByText("ch-7")).toBeInTheDocument();
    expect(screen.getAllByText(/Encoding comfortably ahead/)).toHaveLength(2);
  });

  it("shows a caution badge for a stalled channel", () => {
    render(
      <PlayoutHealthPanel doctor={doctor({ channels: [channel({ health: "stalled", speed: 0.7 })] })} />,
    );
    expect(screen.getByText("stalled")).toBeInTheDocument();
  });

  it("renders the measured realtime speed", () => {
    render(<PlayoutHealthPanel doctor={doctor({ channels: [channel({ speed: 3.2 })] })} />);
    expect(screen.getByText("3.2× rt")).toBeInTheDocument();
  });

  // Direct-play channels report 0 rather than a rate; "0.0× rt" would misread as stalled.
  it("shows a dash rather than zero for a direct-play channel", () => {
    render(<PlayoutHealthPanel doctor={doctor({ channels: [channel({ speed: 0 })] })} />);
    expect(screen.getByText("—")).toBeInTheDocument();
    expect(screen.queryByText("0.0× rt")).not.toBeInTheDocument();
  });

  // The whole reason this panel exists: a resident LLM sharing VRAM with an active hardware
  // encode is a silent cause of stutter nothing else on the dashboard surfaces.
  it("shows the contention warning when the GPU is contended", () => {
    render(
      <PlayoutHealthPanel
        doctor={doctor({
          gpu: { name: "RTX 3080 Ti", vramGiB: 12, llmModel: "qwen3:8b", llmVramGiB: 4.9, contended: true },
          channels: [channel({ health: "degraded" })],
        })}
      />,
    );
    expect(screen.getByText(/LLM sharing VRAM/)).toBeInTheDocument();
    expect(screen.getByText(/qwen3:8b/)).toBeInTheDocument();
  });

  it("does not show the contention warning when the GPU is not contended", () => {
    render(<PlayoutHealthPanel doctor={doctor({ gpu: { vramGiB: 12, contended: false } })} />);
    expect(screen.queryByText(/LLM sharing VRAM/)).not.toBeInTheDocument();
  });

  it("says software encoding when no GPU is detected", () => {
    render(<PlayoutHealthPanel doctor={doctor({ gpu: { vramGiB: 0, contended: false } })} />);
    expect(screen.getByText(/Software encoding \(no GPU\)/)).toBeInTheDocument();
  });

  // ⚠ THE distinction the backend sends `running` for. No internal playout to diagnose is not
  // the same as every channel being unhealthy.
  it("explains a Tunarr-backed install instead of showing an empty table", () => {
    render(<PlayoutHealthPanel doctor={doctor({ running: false })} />);
    expect(screen.getByText(/Tunarr/)).toBeInTheDocument();
  });

  it("says nothing is being watched when playout is running but idle", () => {
    render(<PlayoutHealthPanel doctor={doctor()} />);
    expect(screen.getByText(/Nothing is being watched/)).toBeInTheDocument();
  });

  it("shows a loading line while the first read is in flight", () => {
    render(<PlayoutHealthPanel loading />);
    expect(screen.getByText(/Reading playout health/)).toBeInTheDocument();
  });
});
