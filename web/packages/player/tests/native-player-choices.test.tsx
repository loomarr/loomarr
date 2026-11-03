// @vitest-environment jsdom

import type { ChannelSubtitlesOutputBody } from "@loomarr/api/models/channelSubtitlesOutputBody";
import type { ChannelTracksOutputBody } from "@loomarr/api/models/channelTracksOutputBody";
import { act, createElement } from "react";
import { createRoot } from "react-dom/client";
import { describe, expect, it, vi } from "vitest";

vi.mock("expo-crypto", () => ({ randomUUID: vi.fn() }));
vi.mock("expo-video", () => ({ createVideoPlayer: vi.fn(), VideoView: vi.fn() }));

const { subtitleLabel, useNativePlayerChoices } = await import("@loomarr/player/native");
type Input = Parameters<typeof useNativePlayerChoices>[0];
type Choices = ReturnType<typeof useNativePlayerChoices>;

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const english = { language: "eng", languageName: "English", title: "" };
const subtitlesWindow: ChannelSubtitlesOutputBody = {
  airings: [
    { cues: [{ endMs: 60_000, startMs: 0, text: "Hello" }], startMs: 0, stopMs: 600_000, tracks: [english] },
  ],
};
const dubbed = {
  audio: [
    { index: 0, language: "spa", languageName: "Español" },
    { index: 1, language: "eng", languageName: "English" },
  ],
  alternateAudio: 0,
  defaultAudio: 1,
} as unknown as ChannelTracksOutputBody;

/** A paired client's choices at a playhead, with a fake player and device storage. */
const mount = async () => {
  const saved = new Map<string, string>();
  const setAlternateAudio = vi.fn();
  const input: Input = {
    client: {
      subtitlesSource: { load: vi.fn().mockResolvedValue(subtitlesWindow) },
      tracks: dubbed,
      transport: {
        getPlayer: () => undefined,
        setAlternateAudio,
        subscribePlayer: () => () => undefined,
      } as unknown as Input["client"]["transport"],
    },
    playingChannelId: "a",
    storage: {
      getItem: async () => null,
      setItem: async (key, value) => {
        saved.set(key, value);
      },
    },
    viewerTimeMs: 10_000,
  };
  const result = { current: undefined as unknown as Choices };
  const Probe = () => {
    result.current = useNativePlayerChoices(input);
    return null;
  };
  const root = createRoot(document.createElement("div"));
  await act(async () => root.render(createElement(Probe)));
  return { result, saved, setAlternateAudio };
};

describe("a native player's choices (#2088 decisions 5 and 6)", () => {
  it("offers Off and the programme's track, captioning the chosen one and remembering it", async () => {
    const { result, saved } = await mount();
    expect(result.current.subtitles?.options).toEqual([
      { label: "Off", selected: true, value: "off" },
      { label: subtitleLabel(english), selected: false, value: "0" },
    ]);
    expect(result.current.captionLines).toEqual([]);

    await act(async () => result.current.subtitles?.onSelect("0"));
    expect(result.current.subtitlesOn).toBe(true);
    expect(result.current.captionLines).toEqual(["Hello"]);
    expect(saved.get("loomarr.player.subtitles")).toContain('"eng"');

    // The transport's Subtitles step goes back to Off.
    await act(async () => result.current.nextSubtitles?.());
    expect(result.current.subtitlesOn).toBe(false);
  });

  it("plays the alternate audio once the viewer chooses its language", async () => {
    const { result, setAlternateAudio } = await mount();
    expect(setAlternateAudio).toHaveBeenLastCalledWith(false);
    await act(async () => result.current.audio.onSelect?.("spa"));
    expect(setAlternateAudio).toHaveBeenLastCalledWith(true);
    expect(result.current.quality.options).toEqual([{ label: "Auto", selected: true, value: "auto" }]);
  });
});
