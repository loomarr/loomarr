// @vitest-environment jsdom

import type { ChannelSubtitlesSource } from "@loomarr/core/channel-subtitles";
import type { PlayerSnapshot } from "@loomarr/player";
import { act, createElement } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, describe, expect, it, vi } from "vitest";

vi.mock("expo-crypto", () => ({ randomUUID: vi.fn() }));
vi.mock("expo-video", () => ({ createVideoPlayer: vi.fn(), VideoView: vi.fn() }));

const { usePairedSubtitles } = await import("@loomarr/paired-client");
type PairedSubtitles = ReturnType<typeof usePairedSubtitles>;

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const channel = { id: "ch-1", inAppPlayable: true, name: "Science Fiction", number: 7 };
const playingAt = (viewerTimeMs: number): PlayerSnapshot => ({
  catalog: [channel],
  channel,
  livePlayback: { lagSeconds: 0, mode: "live", noticeRevision: 0, viewerTimeMs },
  recentChannelIds: [],
  status: "playing",
});

// The programme has English; each ask's cue at 1,000,000 names the track it was asked for.
const source = (): ChannelSubtitlesSource & { asked: (string | undefined)[] } => {
  const asked: (string | undefined)[] = [];
  return {
    asked,
    load: async (_channelId, { track }) => {
      asked.push(track);
      return {
        cues: track ? [{ endMs: 1_004_000, startMs: 1_000_000, text: `In ${track}` }] : [],
        pending: false,
        tracks: [{ forced: false, hearingImpaired: false, key: "en", language: "en", title: "" }],
      };
    },
  };
};

const storageWith = (stored: string | null) => ({
  getItem: vi.fn(async () => stored),
  setItem: vi.fn(async () => undefined),
});

let seen: PairedSubtitles | undefined;
const Probe = (props: {
  snapshot: PlayerSnapshot;
  source: ChannelSubtitlesSource;
  storage: ReturnType<typeof storageWith>;
}) => {
  seen = usePairedSubtitles(props.source, props.storage, props.snapshot);
  return null;
};

const roots: ReturnType<typeof createRoot>[] = [];
const mount = () => {
  const root = createRoot(document.createElement("div"));
  roots.push(root);
  return (props: Parameters<typeof Probe>[0]) => act(async () => root.render(createElement(Probe, props)));
};

afterEach(() => {
  for (const root of roots.splice(0)) act(() => root.unmount());
  seen = undefined;
});

describe("usePairedSubtitles", () => {
  it("starts Off, reading the programme's tracks but no track's lines", async () => {
    const src = source();
    await mount()({ snapshot: playingAt(1_001_000), source: src, storage: storageWith(null) });
    expect(seen?.options.map((o) => [o.label, o.selected])).toEqual([
      ["Off", true],
      ["English", false],
    ]);
    expect(seen?.captions).toEqual([]);
    expect(src.asked).toEqual([undefined]);
  });

  it("draws the remembered track's line at the picture's clock", async () => {
    const render = mount();
    const src = source();
    await render({ snapshot: playingAt(1_001_000), source: src, storage: storageWith("en") });
    await act(async () => undefined);
    expect(seen?.on).toBe(true);
    expect(seen?.captions).toEqual(["In en"]);
  });

  it("remembers a choice on the device, and C's next steps round to Off", async () => {
    const storage = storageWith(null);
    await mount()({ snapshot: playingAt(1_001_000), source: source(), storage });
    await act(async () => seen?.choose("en"));
    expect(storage.setItem).toHaveBeenCalledWith("loomarr.subtitles", "en");
    expect(seen?.captions).toEqual(["In en"]);
    await act(async () => seen?.next());
    expect(storage.setItem).toHaveBeenLastCalledWith("loomarr.subtitles", "off");
    expect(seen?.captions).toEqual([]);
  });

  it("reads nothing while no channel plays", async () => {
    const src = source();
    await mount()({
      snapshot: { catalog: [], recentChannelIds: [], status: "idle" },
      source: src,
      storage: storageWith("en"),
    });
    expect(src.asked).toEqual([]);
    expect(seen?.captions).toEqual([]);
  });
});
