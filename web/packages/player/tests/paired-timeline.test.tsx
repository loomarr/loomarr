// @vitest-environment jsdom

import type { ChannelTimeline, ChannelTimelineSource } from "@loomarr/core/channel-timeline";
import { act, createElement } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, describe, expect, it, vi } from "vitest";

import { TIMELINE_REFRESH_MS, usePairedTimeline } from "../src/paired-timeline";

const timelineOf = (serverNowMs: number): ChannelTimeline => ({ airings: [], receivedAtMs: 0, serverNowMs });

// Renders the hook for a channel and reports each value it returns.
const mount = async (source: ChannelTimelineSource, channelId: string | undefined) => {
  const seen: (ChannelTimeline | undefined)[] = [];
  const Probe = ({ id }: { id: string | undefined }) => {
    seen.push(usePairedTimeline(source, id));
    return null;
  };
  const root = createRoot(document.createElement("div"));
  await act(async () => root.render(createElement(Probe, { id: channelId })));
  return {
    current: () => seen.at(-1),
    retune: (id: string | undefined) => act(async () => root.render(createElement(Probe, { id }))),
    unmount: () => act(async () => root.unmount()),
  };
};

describe("the tuned channel's timeline (#2127)", () => {
  afterEach(() => vi.useRealTimers());

  it("loads for the tuned channel, refreshes while it stays tuned, and stops when unmounted", async () => {
    vi.useFakeTimers();
    const load = vi.fn().mockResolvedValueOnce(timelineOf(1)).mockResolvedValueOnce(timelineOf(2));
    const probe = await mount({ load }, "a");
    expect(load).toHaveBeenCalledWith("a", expect.any(AbortSignal));
    expect(probe.current()?.serverNowMs).toBe(1);

    await act(async () => vi.advanceTimersByTime(TIMELINE_REFRESH_MS));
    expect(probe.current()?.serverNowMs).toBe(2);

    await probe.unmount();
    await act(async () => vi.advanceTimersByTime(TIMELINE_REFRESH_MS));
    expect(load).toHaveBeenCalledTimes(2);
  });

  it("never shows one channel's schedule under another while the new one loads", async () => {
    let answerB: (timeline: ChannelTimeline) => void = () => undefined;
    const load = vi.fn((id: string) =>
      id === "a" ? Promise.resolve(timelineOf(1)) : new Promise<ChannelTimeline>((r) => (answerB = r)),
    );
    const probe = await mount({ load }, "a");
    expect(probe.current()?.serverNowMs).toBe(1);

    await probe.retune("b");
    expect(probe.current()).toBeUndefined();
    await act(async () => answerB(timelineOf(9)));
    expect(probe.current()?.serverNowMs).toBe(9);
    await probe.unmount();
  });

  it("keeps the last answer through a failed refresh, and asks for nothing with no channel", async () => {
    vi.useFakeTimers();
    const load = vi.fn().mockResolvedValueOnce(timelineOf(1)).mockRejectedValueOnce(new Error("502"));
    const probe = await mount({ load }, "a");
    await act(async () => vi.advanceTimersByTime(TIMELINE_REFRESH_MS));
    expect(probe.current()?.serverNowMs).toBe(1);
    await probe.unmount();

    const idle = vi.fn();
    const none = await mount({ load: idle }, undefined);
    expect(none.current()).toBeUndefined();
    expect(idle).not.toHaveBeenCalled();
    await none.unmount();
  });
});
