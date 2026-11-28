// @vitest-environment jsdom

import type { AudioChoice, ChannelAudioSource } from "@loomarr/core/channel-audio";
import { act, cleanup, renderUi } from "@loomarr/test-support";
import { createElement } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

vi.mock("expo-crypto", () => ({ randomUUID: vi.fn() }));
vi.mock("expo-video", () => ({ createVideoPlayer: vi.fn(), VideoView: vi.fn() }));

const { useChannelAudio } = await import("@loomarr/paired-client");

const english: AudioChoice[] = [{ label: "English", selected: true, value: "eng" }];

/** A source whose reads the test settles by hand, recording each read's channel and signal. */
const deferredSource = () => {
  const reads: { channelId: string; resolve: (c: AudioChoice[]) => void; signal: AbortSignal }[] = [];
  const source: ChannelAudioSource = {
    load: (channelId, signal) =>
      new Promise((resolve) => {
        reads.push({ channelId, resolve, signal });
      }),
  };
  return { reads, source };
};

let seen: readonly AudioChoice[] | undefined;
const Probe = (props: { channelId?: string; open: boolean; source: ChannelAudioSource }) => {
  seen = useChannelAudio(props.source, props.channelId, props.open);
  return null;
};

const mount = () => {
  const { rerender } = renderUi(null);
  return (props: Parameters<typeof Probe>[0]) => rerender(createElement(Probe, props));
};

afterEach(() => {
  cleanup();
  seen = undefined;
});

describe("useChannelAudio", () => {
  it("reads nothing while the menu is closed", async () => {
    const { reads, source } = deferredSource();
    await mount()({ channelId: "ch-1", open: false, source });
    expect(reads).toHaveLength(0);
    expect(seen).toBeUndefined();
  });

  it("lists the channel's audio once the read lands", async () => {
    const { reads, source } = deferredSource();
    await mount()({ channelId: "ch-1", open: true, source });
    expect(seen).toBeUndefined();
    await act(async () => reads[0]?.resolve(english));
    expect(seen).toEqual(english);
  });

  it("drops the read when the channel changes, and never shows the old channel's rows", async () => {
    const { reads, source } = deferredSource();
    const render = mount();
    await render({ channelId: "ch-1", open: true, source });
    await render({ channelId: "ch-2", open: true, source });
    expect(reads[0]?.signal.aborted).toBe(true);
    await act(async () => reads[0]?.resolve(english));
    expect(seen).toBeUndefined();
    expect(reads[1]?.channelId).toBe("ch-2");
  });

  it("reads afresh each time the menu opens", async () => {
    const { reads, source } = deferredSource();
    const render = mount();
    await render({ channelId: "ch-1", open: true, source });
    await act(async () => reads[0]?.resolve(english));
    await render({ channelId: "ch-1", open: false, source });
    expect(seen).toBeUndefined();
    await render({ channelId: "ch-1", open: true, source });
    expect(reads).toHaveLength(2);
  });
});
