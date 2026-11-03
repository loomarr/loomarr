// @vitest-environment jsdom

import { act, createElement } from "react";
import { createRoot } from "react-dom/client";
import { describe, expect, it, vi } from "vitest";

vi.mock("expo-crypto", () => ({ randomUUID: vi.fn() }));
vi.mock("expo-video", () => ({ createVideoPlayer: vi.fn(), VideoView: vi.fn() }));

const { useAudioPreference, useSubtitlesPreference } = await import("@loomarr/player/native");
type Storage = Parameters<typeof useAudioPreference>[0];

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

/** Device storage the test answers when it chooses, holding what was set. */
const deferredStorage = (stored: string | null) => {
  const saved = new Map<string, string>();
  let answer: (value: string | null) => void = () => undefined;
  const storage: Storage = {
    getItem: () =>
      new Promise((resolve) => {
        answer = resolve;
      }),
    setItem: async (key, value) => {
      saved.set(key, value);
    },
  };
  return { answer: () => act(async () => answer(stored)), saved, storage };
};

/** Renders a preference hook, keeping its latest value and setter. */
const probe = <T,>(use: () => readonly [T, (next: T) => void]) => {
  const result = { current: undefined as unknown as readonly [T, (next: T) => void] };
  const Probe = () => {
    result.current = use();
    return null;
  };
  const container = (
    globalThis as unknown as {
      document: { createElement: (tag: string) => Parameters<typeof createRoot>[0] };
    }
  ).document.createElement("div");
  act(() => createRoot(container).render(createElement(Probe)));
  return result;
};

describe("the viewer's player preferences (#2088 decisions 5 and 6)", () => {
  it("reads as the default until storage answers, then as what it remembered", async () => {
    const device = deferredStorage(JSON.stringify("fr"));
    const audio = probe(() => useAudioPreference(device.storage));
    expect(audio.current[0]).toBe("default");
    await device.answer();
    expect(audio.current[0]).toBe("fr");
  });

  it("keeps a choice made before storage answers, and remembers it", async () => {
    const device = deferredStorage(JSON.stringify("off"));
    const subtitles = probe(() => useSubtitlesPreference(device.storage));
    const track = { language: "fr", title: "Français" };
    act(() => subtitles.current[1](track));
    await device.answer();
    expect(subtitles.current[0]).toEqual(track);
    expect(device.saved.get("loomarr.player.subtitles")).toBe(JSON.stringify(track));
  });

  it("stays the default when what is stored isn't a choice", async () => {
    const device = deferredStorage("{not json");
    const audio = probe(() => useAudioPreference(device.storage));
    await device.answer();
    expect(audio.current[0]).toBe("default");
  });
});
