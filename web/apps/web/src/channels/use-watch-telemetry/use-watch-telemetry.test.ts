import type { GuideAiring } from "@loomarr/api/models/guideAiring";
import { GuideAiringKind } from "@loomarr/api/models/guideAiringKind";
import { renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { clientDiagnostics } from "@/diagnostics/client-reporter";
import { useWatchTelemetry } from "./use-watch-telemetry";

const airing = (scheduleBlockId: string, startMs: number, stopMs: number, extra: Partial<GuideAiring> = {}) =>
  ({ scheduleBlockId, startMs, stopMs, kind: "program", title: scheduleBlockId, ...extra }) as GuideAiring;

const airings = [airing("b1", 0, 1_000), airing("b2", 1_000, 2_000, { kind: GuideAiringKind.pending })];

type Props = Parameters<typeof useWatchTelemetry>[0];
const base: Props = {
  channelId: "ch-1",
  playbackSessionId: "s-1",
  airings,
  viewerTimeMs: 500,
  serverNowMs: 500,
};

describe("useWatchTelemetry", () => {
  const record = vi.spyOn(clientDiagnostics, "record");
  beforeEach(() => {
    record.mockReset().mockImplementation(() => {});
  });

  const events = (name: string) => record.mock.calls.map(([e]) => e).filter((e) => e.event === name);

  it("reports each block the picture enters, with the one it left", () => {
    const hook = renderHook((props: Props) => useWatchTelemetry(props), { initialProps: base });
    hook.rerender({ ...base, viewerTimeMs: 600, serverNowMs: 600 });
    hook.rerender({ ...base, viewerTimeMs: 1_500, serverNowMs: 1_500 });
    expect(events("player.schedule_block_changed")).toEqual([
      expect.objectContaining({
        scheduleBlockId: "b1",
        previousScheduleBlockId: undefined,
        blockKind: "program",
      }),
      expect.objectContaining({ scheduleBlockId: "b2", previousScheduleBlockId: "b1", blockKind: "flex" }),
    ]);
  });

  it("says nothing before the server's first read, or on a nominal airing", () => {
    renderHook(() => useWatchTelemetry({ ...base, serverNowMs: undefined }));
    renderHook(() => useWatchTelemetry({ ...base, airings: [airing("b1", 0, 1_000, { nominal: true })] }));
    expect(record).not.toHaveBeenCalled();
  });

  it("reports a half-minute drift from the live edge once per block", () => {
    const behind = { ...base, serverNowMs: 30_500 };
    const hook = renderHook((props: Props) => useWatchTelemetry(props), { initialProps: behind });
    hook.rerender({ ...behind, viewerTimeMs: 600, serverNowMs: 40_000 });
    expect(events("player.playhead_drift")).toEqual([expect.objectContaining({ driftMs: -30_000 })]);
  });

  it("does not call a playhead under half a minute out a drift", () => {
    renderHook(() => useWatchTelemetry({ ...base, serverNowMs: 30_499 }));
    expect(events("player.playhead_drift")).toEqual([]);
  });

  it("starts afresh on another channel", () => {
    const behind = { ...base, serverNowMs: 30_500 };
    const hook = renderHook((props: Props) => useWatchTelemetry(props), { initialProps: behind });
    hook.rerender({ ...behind, channelId: "ch-2" });
    expect(events("player.schedule_block_changed")).toEqual([
      expect.objectContaining({ channelId: "ch-1", scheduleBlockId: "b1" }),
      expect.objectContaining({
        channelId: "ch-2",
        scheduleBlockId: "b1",
        previousScheduleBlockId: undefined,
      }),
    ]);
    expect(events("player.playhead_drift")).toHaveLength(2);
  });
});
