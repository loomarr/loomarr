// @vitest-environment jsdom

import type { VideoPlayer } from "expo-video";
import { act, createElement, useEffect } from "react";
import { createRoot } from "react-dom/client";
import { describe, expect, it, vi } from "vitest";

vi.mock("expo-crypto", () => ({ randomUUID: vi.fn() }));
vi.mock("expo-video", () => ({ createVideoPlayer: vi.fn(), VideoView: vi.fn() }));

const { createNativePlayerTransport, useNativeVideoHeight } = await import("@loomarr/player/native");

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

/** A player that, like Expo's, throws when read once released. */
const nativePlayer = (height: number) => {
  let released = false;
  const live = () => {
    if (released) throw new Error("Unable to find the native shared object");
  };
  return {
    addListener: () => ({ remove: live }),
    get availableAudioTracks() {
      live();
      return [];
    },
    pause: () => undefined,
    release: () => {
      released = true;
    },
    get videoTrack() {
      live();
      return { size: { height, width: (height * 16) / 9 } };
    },
  } as unknown as VideoPlayer;
};

describe("the rendition on the picture (#2127)", () => {
  // A launch in the background (tvOS relaunching a paired TV) suspends the transport in an earlier
  // effect, after this hook rendered with the first player and before its own effect runs.
  it("never reads a player released between render and effect", async () => {
    const transport = createNativePlayerTransport(nativePlayer(720), () => nativePlayer(1080));
    let height: number | undefined;
    const Probe = () => {
      useEffect(() => transport.suspend(), []);
      height = useNativeVideoHeight(transport);
      return null;
    };
    const root = createRoot(document.createElement("div"));
    await act(async () => root.render(createElement(Probe)));
    expect(height).toBeUndefined();

    await act(async () => transport.resume());
    expect(height).toBe(1080);
    await act(async () => root.unmount());
  });
});
