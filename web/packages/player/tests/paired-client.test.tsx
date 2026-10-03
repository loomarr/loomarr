// @vitest-environment jsdom

import type { PairingCredential, PairingSession } from "@loomarr/core/pairing";
import type { PlayerChannel, PlayerTransport, PlayerTransportEvent } from "@loomarr/player";
import { act, createElement } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, describe, expect, it, vi } from "vitest";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const channels: PlayerChannel[] = [
  { id: "ten", inAppPlayable: true, name: "Ten", number: 10 },
  { id: "twenty", inAppPlayable: true, name: "Twenty", number: 20 },
];

const fakes = vi.hoisted(() => {
  const emptySnapshot = {};
  return {
    emptySnapshot,
    guide: {
      dispose: () => undefined,
      getSnapshot: () => emptySnapshot,
      refresh: async () => undefined,
      subscribe: () => () => undefined,
    },
    myChannels: {
      dispose: () => undefined,
      getSnapshot: () => emptySnapshot,
      recordTune: undefined as unknown as (channelId: string) => Promise<void>,
      refresh: async () => undefined,
      subscribe: () => () => undefined,
    },
    playbackSessionIds: [] as string[],
    transport: undefined as unknown as PlayerTransport & { emit: (event: PlayerTransportEvent) => void },
  };
});

vi.mock("expo-crypto", () => ({ randomUUID: () => "11111111-2222-4333-8444-555555555555" }));
vi.mock("expo-video", () => ({ createVideoPlayer: vi.fn(), VideoView: vi.fn() }));
vi.mock("@loomarr/core/events", () => ({ openEventStream: () => () => undefined }));
vi.mock("@loomarr/core/guide", () => ({
  createGuideController: () => fakes.guide,
  createGuideSourcePort: () => ({}),
  guideWindow: vi.fn(),
}));
vi.mock("@loomarr/core/my-channels", () => ({
  createMyChannelsController: () => fakes.myChannels,
  createMyChannelsPort: () => ({}),
}));
vi.mock("@loomarr/core/client-diagnostics", () => ({
  ClientDiagnosticsReporter: class {
    dispose = () => undefined;
    wireBatch = () => ({});
  },
  createAuthenticatedBatchSender: () => ({}),
}));
vi.mock("../src/native-event-stream", () => ({ createNativeEventStreamFactory: () => vi.fn() }));
vi.mock("../src/native-playback-diagnostics", () => ({
  createNativePlaybackDiagnostics: (_reporter: unknown, sessionId: string) => {
    fakes.playbackSessionIds.push(sessionId);
    return {
      channelChanged: () => undefined,
      dispose: () => undefined,
      playerError: () => undefined,
      transportEvent: () => undefined,
    };
  },
}));
vi.mock("../src/native/native", () => ({ createExpoVideoTransport: () => fakes.transport }));
vi.mock("../src/play-url-source", () => ({
  createChannelCatalogPort: () => ({ list: async () => channels }),
  createPlayUrlSourcePort: () => ({
    mint: async (channel: PlayerChannel) => ({ uri: `${channel.id}.m3u8` }),
  }),
}));

const { usePairedClient } = await import("@loomarr/player/native");
type PairedClient = ReturnType<typeof usePairedClient>;

const credential = { deviceName: "Test", serverUrl: "https://loomarr.test", token: "t" } as PairingCredential;
const session = { revoked: () => undefined } as unknown as PairingSession;

const mountClient = async (diagnostics?: { platform: string }) => {
  const listeners = new Set<(event: PlayerTransportEvent) => void>();
  fakes.transport = {
    dispose: vi.fn(),
    emit: (event) => {
      for (const listener of listeners) listener(event);
    },
    goLive: vi.fn(),
    pause: vi.fn(),
    play: vi.fn(),
    replace: vi.fn().mockResolvedValue(undefined),
    subscribe: (listener) => {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
  };
  const recordTune = vi.fn().mockResolvedValue(undefined);
  fakes.myChannels.recordTune = recordTune;
  let client!: PairedClient;
  const Probe = () => {
    client = usePairedClient({
      credential,
      diagnostics: diagnostics as never,
      initialTune: "first",
      session,
    });
    return null;
  };
  const container = (
    globalThis as unknown as { document: { createElement: (tag: string) => never } }
  ).document.createElement("div");
  const root = createRoot(container);
  await act(async () => root.render(createElement(Probe)));
  const settle = (event: PlayerTransportEvent["type"]) =>
    act(async () => {
      fakes.transport.emit({
        attemptId: client.controller.getSnapshot().attemptId as number,
        type: event,
      } as PlayerTransportEvent);
    });
  return { client: () => client, recordTune, root, settle };
};

afterEach(() => {
  fakes.playbackSessionIds.length = 0;
});

describe("paired client", () => {
  it("records a recent tune once per channel change, not once per pause and resume", async () => {
    const { client, recordTune, root, settle } = await mountClient();
    expect(client().snapshot).toMatchObject({ channel: { id: "ten" }, status: "tuning" });
    expect(recordTune).not.toHaveBeenCalled();

    await settle("first-frame");
    expect(recordTune.mock.calls).toEqual([["ten"]]);

    await act(async () => client().controller.pause());
    await settle("paused");
    await act(async () => client().controller.play());
    await settle("playing");
    expect(client().snapshot.status).toBe("playing");
    expect(recordTune.mock.calls).toEqual([["ten"]]);

    await act(async () => client().controller.tuneChannel("twenty"));
    await settle("first-frame");
    expect(recordTune.mock.calls).toEqual([["ten"], ["twenty"]]);
    await act(async () => root.unmount());
  });

  it("names the native playback session '<platform>-<uuid>' from expo-crypto", async () => {
    const { root } = await mountClient({ platform: "ios" });
    expect(fakes.playbackSessionIds).toEqual(["ios-11111111-2222-4333-8444-555555555555"]);
    await act(async () => root.unmount());
  });
});
