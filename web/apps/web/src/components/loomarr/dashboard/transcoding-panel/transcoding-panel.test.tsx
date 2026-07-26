import type { PlayoutTelemetry } from "@loomarr/api";
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { TranscodingPanel } from "./transcoding-panel";

const telemetry = (over: Partial<PlayoutTelemetry> = {}): PlayoutTelemetry =>
  ({ running: true, active: 0, capacity: 4, sessions: [], ...over }) as PlayoutTelemetry;

const session = (over: Record<string, unknown> = {}) => ({
  channelId: "ch-42",
  viewers: 2,
  encoder: "h264_nvenc",
  hardware: true,
  speed: 12.4,
  bufferedMs: 96_000,
  uptimeMs: 720_000,
  ...over,
});

describe("TranscodingPanel", () => {
  it("shows a row per stream with its load line", () => {
    render(
      <TranscodingPanel
        telemetry={telemetry({ active: 2, sessions: [session(), session({ channelId: "ch-7" })] })}
      />,
    );
    expect(screen.getByText("ch-42")).toBeInTheDocument();
    expect(screen.getByText("2 / 4")).toBeInTheDocument();
  });

  // The most actionable field on the panel: hardware vs software is the difference between
  // four concurrent streams and one.
  it("distinguishes hardware from software encoding", () => {
    render(
      <TranscodingPanel
        telemetry={telemetry({
          sessions: [session(), session({ channelId: "ch-7", hardware: false, encoder: "libx264" })],
        })}
      />,
    );
    expect(screen.getByText("Hardware")).toBeInTheDocument();
    expect(screen.getByText("Software")).toBeInTheDocument();
  });

  it("renders the measured realtime speed", () => {
    render(<TranscodingPanel telemetry={telemetry({ sessions: [session({ speed: 12.4 })] })} />);
    expect(screen.getByText("12.4× rt")).toBeInTheDocument();
  });

  // A fresh encoder has not reported yet. "0× rt" would read as stalled when it means
  // "no sample", so the slot shows an em dash instead.
  it("shows a dash rather than zero before the first progress sample", () => {
    render(<TranscodingPanel telemetry={telemetry({ sessions: [session({ speed: 0 })] })} />);
    expect(screen.getByText("—")).toBeInTheDocument();
    expect(screen.queryByText("0.0× rt")).not.toBeInTheDocument();
  });

  // Negative buffer is the encoder LOSING to realtime — the same condition a sub-1.0 speed
  // reports, expressed as accumulated deficit.
  it("says an encoder is behind rather than showing a negative buffer", () => {
    render(<TranscodingPanel telemetry={telemetry({ sessions: [session({ bufferedMs: -4000 })] })} />);
    expect(screen.getByText(/4s behind/)).toBeInTheDocument();
  });

  it("reports how far ahead a healthy encoder has buffered", () => {
    render(<TranscodingPanel telemetry={telemetry({ sessions: [session({ bufferedMs: 96_000 })] })} />);
    expect(screen.getByText(/encoded 96s ahead/)).toBeInTheDocument();
  });

  // ⚠ THE distinction the backend sends `running` for. An empty list on a Tunarr-backed
  // install means "not our job", not "every channel died".
  it("explains a Tunarr-backed install instead of showing an empty table", () => {
    render(<TranscodingPanel telemetry={telemetry({ running: false, capacity: 0 })} />);
    expect(screen.getByText(/Tunarr/)).toBeInTheDocument();
    // …and no load line, which would read as 0 of 0 capacity.
    expect(screen.queryByText("0 / 0")).not.toBeInTheDocument();
  });

  it("says nothing is being watched when playout is running but idle", () => {
    render(<TranscodingPanel telemetry={telemetry()} />);
    expect(screen.getByText(/Nothing is being watched/)).toBeInTheDocument();
  });

  it("shows a loading line while the first read is in flight", () => {
    render(<TranscodingPanel loading />);
    expect(screen.getByText(/Reading encoder state/)).toBeInTheDocument();
  });
});
