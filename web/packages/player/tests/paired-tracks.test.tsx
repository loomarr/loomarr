// @vitest-environment jsdom

import type { ChannelTracksOutputBody } from "@loomarr/api/models/channelTracksOutputBody";
import type { ChannelTracksSource } from "@loomarr/core/channel-tracks";
import { act, createElement } from "react";
import { createRoot } from "react-dom/client";
import { describe, expect, it, vi } from "vitest";

import { usePairedTracks } from "../src/paired-tracks";

const tracksIn = (language: string): ChannelTracksOutputBody => ({
  audio: [{ index: 1, language }],
  defaultAudio: 1,
  subtitles: [],
});

// Renders the hook for a channel's programme and reports each value it returns.
const mount = async (source: ChannelTracksSource, channelId: string | undefined, startMs?: number) => {
  const seen: (ChannelTracksOutputBody | undefined)[] = [];
  const Probe = ({ id, at }: { id: string | undefined; at?: number }) => {
    seen.push(usePairedTracks(source, id, at));
    return null;
  };
  const root = createRoot(document.createElement("div"));
  await act(async () => root.render(createElement(Probe, { at: startMs, id: channelId })));
  return {
    current: () => seen.at(-1),
    move: (id: string | undefined, at?: number) =>
      act(async () => root.render(createElement(Probe, { at, id }))),
    unmount: () => act(async () => root.unmount()),
  };
};

describe("the tracks of the programme at the playhead (#2127)", () => {
  it("asks once per programme, by its start", async () => {
    const load = vi.fn().mockResolvedValue(tracksIn("eng"));
    const probe = await mount({ load }, "a", 1_000);
    expect(load).toHaveBeenCalledWith("a", 1_000, expect.any(AbortSignal));
    expect(probe.current()?.audio[0]?.language).toBe("eng");

    await probe.move("a", 1_000);
    expect(load).toHaveBeenCalledOnce();
    await probe.unmount();
  });

  it("never offers one programme's languages over the next while it loads", async () => {
    let answerNext: (tracks: ChannelTracksOutputBody) => void = () => undefined;
    const load = vi.fn((_id: string, at: number) =>
      at === 1_000
        ? Promise.resolve(tracksIn("eng"))
        : new Promise<ChannelTracksOutputBody>((r) => (answerNext = r)),
    );
    const probe = await mount({ load }, "a", 1_000);

    await probe.move("a", 2_000);
    expect(probe.current()).toBeUndefined();
    await act(async () => answerNext(tracksIn("spa")));
    expect(probe.current()?.audio[0]?.language).toBe("spa");
    await probe.unmount();
  });

  it("asks nothing without a programme", async () => {
    const load = vi.fn();
    const probe = await mount({ load }, "a");
    expect(load).not.toHaveBeenCalled();
    expect(probe.current()).toBeUndefined();
    await probe.unmount();
  });
});
