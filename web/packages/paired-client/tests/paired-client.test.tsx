// @vitest-environment jsdom

import type { PairingCredential, PairingSession } from "@loomarr/core/pairing";
import type { LastChannelStore } from "@loomarr/paired-client";
import type { PlayerChannel, PlayerTransport, PlayerTransportEvent } from "@loomarr/player";
import { act, createElement } from "react";
import { createRoot } from "react-dom/client";
import { AppState } from "react-native";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const channels: PlayerChannel[] = [
  { id: "ten", inAppPlayable: true, name: "Ten", number: 10 },
  { id: "twenty", inAppPlayable: true, name: "Twenty", number: 20 },
];

type Request = (input: string, init?: RequestInit) => Promise<Response>;

const fakes = vi.hoisted(() => {
  const emptySnapshot = {};
  return {
    batchSenders: [] as unknown[],
    catalogFails: false,
    catalogHeld: false,
    catalogLists: [] as AbortSignal[],
    emptySnapshot,
    guide: {
      dispose: vi.fn(() => void fakes.lifetimes.push("guide:dispose")),
      getSnapshot: () => emptySnapshot,
      refresh: vi.fn(async () => void fakes.lifetimes.push("guide:use")),
      subscribe: () => () => undefined,
    },
    // The order the guide and my-channels controllers are made, used and disposed in.
    lifetimes: [] as string[],
    guideOptions: [] as { resolveWindow?: (at: Date) => unknown }[],
    guideWindow: vi.fn((window: unknown) => window),
    myChannels: {
      dispose: () => void fakes.lifetimes.push("myChannels:dispose"),
      getSnapshot: () => emptySnapshot,
      recordTune: undefined as unknown as (channelId: string) => Promise<void>,
      refresh: async () => void fakes.lifetimes.push("myChannels:use"),
      subscribe: () => () => undefined,
    },
    playback: {
      channelChanged: vi.fn(),
      dispose: vi.fn(),
      playerError: vi.fn(),
      transportEvent: vi.fn(),
    },
    playbackSessionIds: [] as string[],
    playbackTransports: [] as string[],
    reporterDisposed: vi.fn(),
    requests: [] as unknown[],
    streamOptions: [] as { headers: Record<string, string>; onUnauthorized: () => unknown }[],
    streams: [] as {
      closed: boolean;
      handlers: { onChannel: () => void };
      onOpen?: () => void;
      url: string;
    }[],
    transport: undefined as unknown as PlayerTransport & {
      emit: (event: PlayerTransportEvent) => void;
      resume: ReturnType<typeof vi.fn>;
      suspend: ReturnType<typeof vi.fn>;
    },
  };
});

vi.mock("expo-crypto", () => ({ randomUUID: () => "11111111-2222-4333-8444-555555555555" }));
vi.mock("expo-video", () => ({ createVideoPlayer: vi.fn(), VideoView: vi.fn() }));
vi.mock("@loomarr/core/events", () => ({
  openEventStream: (
    handlers: { onChannel: () => void },
    url: string,
    _createStream: unknown,
    onOpen?: () => void,
  ) => {
    const stream = { closed: false, handlers, onOpen, url };
    fakes.streams.push(stream);
    return () => {
      stream.closed = true;
    };
  },
}));
vi.mock("@loomarr/core/guide", () => ({
  createGuideController: (options: { resolveWindow?: (at: Date) => unknown }) => {
    fakes.guideOptions.push(options);
    fakes.lifetimes.push("guide:create");
    return fakes.guide;
  },
  createGuideSourcePort: () => ({}),
  guideWindow: fakes.guideWindow,
}));
vi.mock("@loomarr/core/my-channels", () => ({
  createMyChannelsController: () => {
    fakes.lifetimes.push("myChannels:create");
    return fakes.myChannels;
  },
  createMyChannelsPort: () => ({}),
}));
vi.mock("@loomarr/core/client-diagnostics", () => ({
  ClientDiagnosticsReporter: class {
    dispose = fakes.reporterDisposed;
    wireBatch = () => ({});
  },
  createAuthenticatedBatchSender: (request: unknown) => {
    fakes.batchSenders.push(request);
    return {};
  },
}));
vi.mock("@loomarr/player/native", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@loomarr/player/native")>()),
  createExpoVideoTransport: () => fakes.transport,
  createNativeEventStreamFactory: (options: {
    headers: Record<string, string>;
    onUnauthorized: () => unknown;
  }) => {
    fakes.streamOptions.push(options);
    return vi.fn();
  },
  createNativePlaybackDiagnostics: (_reporter: unknown, sessionId: string, transport: string) => {
    fakes.playbackSessionIds.push(sessionId);
    fakes.playbackTransports.push(transport);
    return fakes.playback;
  },
}));
vi.mock("@loomarr/player/server", () => ({
  createChannelCatalogPort: (request: unknown) => {
    fakes.requests.push(request);
    return {
      list: (signal: AbortSignal) => {
        fakes.catalogLists.push(signal);
        // A held load stays pending until it is cancelled, as a request on a dead connection does.
        if (fakes.catalogFails) return Promise.reject(new Error("catalog down"));
        return fakes.catalogHeld
          ? new Promise<never>((_, reject) =>
              signal.addEventListener("abort", () => reject(new Error("cancelled"))),
            )
          : Promise.resolve(channels);
      },
    };
  },
  createPlayUrlSourcePort: () => ({
    mint: async (channel: PlayerChannel) => ({ uri: `${channel.id}.m3u8` }),
  }),
}));

const { phoneClientOptions, retryPairedClient, usePairedClient } = await import("@loomarr/paired-client");
type PairedClient = ReturnType<typeof usePairedClient>;

const credential = { deviceName: "Test", serverUrl: "https://loomarr.test", token: "t" } as PairingCredential;
const revoked = vi.fn();
const session = { revoked } as unknown as PairingSession;

// The app's foreground/background changes, delivered to whoever listens, as the system would.
const appListeners = new Set<(state: string) => void>();
const sendAppState = (state: string) =>
  act(async () => {
    for (const listener of [...appListeners]) listener(state);
  });

const mountClient = async (
  diagnostics?: { platform: string },
  lastChannel?: LastChannelStore,
  extra: Partial<Parameters<typeof usePairedClient>[0]> = {},
) => {
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
    resume: vi.fn(),
    subscribe: (listener) => {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
    suspend: vi.fn(),
  };
  const recordTune = vi.fn().mockResolvedValue(undefined);
  fakes.myChannels.recordTune = recordTune;
  let client!: PairedClient;
  let options = extra;
  const Probe = () => {
    client = usePairedClient({
      credential,
      diagnostics: diagnostics as never,
      initialTune: "first",
      lastChannel,
      session,
      ...options,
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
  // The host passes new options (a new last-channel store, say) without the credential changing.
  const rerender = (next: Partial<Parameters<typeof usePairedClient>[0]>) => {
    options = { ...options, ...next };
    return act(async () => root.render(createElement(Probe)));
  };
  return { client: () => client, recordTune, rerender, root, settle };
};

beforeEach(() => {
  vi.spyOn(AppState, "addEventListener").mockImplementation(((
    _type: string,
    listener: (state: string) => void,
  ) => {
    appListeners.add(listener);
    return { remove: () => appListeners.delete(listener) };
  }) as never);
});

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  appListeners.clear();
  fakes.batchSenders.length = 0;
  fakes.catalogFails = false;
  fakes.catalogHeld = false;
  fakes.catalogLists.length = 0;
  fakes.guideOptions.length = 0;
  fakes.lifetimes.length = 0;
  fakes.playbackSessionIds.length = 0;
  fakes.playbackTransports.length = 0;
  fakes.requests.length = 0;
  fakes.streamOptions.length = 0;
  fakes.streams.length = 0;
  for (const spy of [
    fakes.guide.dispose,
    fakes.guide.refresh,
    fakes.guideWindow,
    fakes.playback.channelChanged,
    fakes.playback.dispose,
    fakes.playback.playerError,
    fakes.playback.transportEvent,
    fakes.reporterDisposed,
    revoked,
  ])
    spy.mockClear();
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

  it("saves the last channel when a tune settles, not while it is still tuning", async () => {
    const save = vi.fn().mockResolvedValue(undefined);
    const { client, root, settle } = await mountClient(undefined, {
      load: () => Promise.resolve(undefined),
      save,
    });
    expect(client().snapshot.status).toBe("tuning");
    expect(save).not.toHaveBeenCalled();

    await settle("first-frame");
    expect(save.mock.calls).toEqual([["ten"]]);
    await act(async () => root.unmount());
  });

  it("names the native playback session '<platform>-<uuid>' from expo-crypto", async () => {
    const { root } = await mountClient({ platform: "ios" });
    expect(fakes.playbackSessionIds).toEqual(["ios-11111111-2222-4333-8444-555555555555"]);
    await act(async () => root.unmount());
  });

  it("labels diagnostics with the transport the platform plays through", async () => {
    const apple = await mountClient({ platform: "apple_tv" });
    await act(async () => apple.root.unmount());
    const android = await mountClient({ platform: "android" });
    await act(async () => android.root.unmount());
    expect(fakes.playbackTransports).toEqual(["native_hls", "media3"]);
  });

  describe("requests", () => {
    it("carry the device's token, and a 401 ends the pairing", async () => {
      // The request takes the platform's fetch when the client is built.
      let status = 200;
      const send = vi.fn(async (_input: string, _init?: RequestInit) => new Response("{}", { status }));
      vi.stubGlobal("fetch", send);
      const { root } = await mountClient();
      send.mockClear();
      revoked.mockClear();
      status = 401;

      await (fakes.requests[0] as Request)("https://loomarr.test/v1/channels");

      const headers = send.mock.calls[0]?.[1]?.headers as Headers;
      expect(headers.get("Authorization")).toBe("Bearer t");
      expect(revoked).toHaveBeenCalledTimes(1);
      await act(async () => root.unmount());
    });

    it("never leave the paired server", async () => {
      const send = vi.fn(async () => new Response("{}", { status: 200 }));
      vi.stubGlobal("fetch", send);
      const { root } = await mountClient();
      send.mockClear();

      await expect((fakes.requests[0] as Request)("https://elsewhere.test/v1/channels")).rejects.toThrow(
        /cannot leave the paired server origin/,
      );
      expect(send).not.toHaveBeenCalled();
      await act(async () => root.unmount());
    });
  });

  describe("the live event stream", () => {
    it("opens on the server's events endpoint with the device's token, and a 401 ends the pairing", async () => {
      const { root } = await mountClient();

      expect(fakes.streams.map(({ url }) => url)).toEqual(["https://loomarr.test/v1/events"]);
      expect(fakes.streamOptions[0]?.headers).toEqual({ Authorization: "Bearer t" });
      fakes.streamOptions[0]?.onUnauthorized();
      expect(revoked).toHaveBeenCalledTimes(1);
      await act(async () => root.unmount());
    });

    it("reloads the catalog and the guide when the server announces a channel change", async () => {
      const { root } = await mountClient();
      expect(fakes.catalogLists).toHaveLength(1);
      fakes.guide.refresh.mockClear();

      await act(async () => fakes.streams[0]?.handlers.onChannel());

      expect(fakes.catalogLists).toHaveLength(2);
      expect(fakes.guide.refresh).toHaveBeenCalledWith();
      await act(async () => root.unmount());
    });

    // Frames sent while the stream was down (a server restarting for an update) are gone, so a
    // reconnect reloads what they would have changed. The first open needs none: the catalog and
    // guide load as the stream opens, on launch and on every return to the foreground.
    it("reloads the catalog and the guide when the stream comes back, not when it first opens", async () => {
      const { root } = await mountClient();
      expect(fakes.catalogLists).toHaveLength(1);
      fakes.guide.refresh.mockClear();

      await act(async () => fakes.streams[0]?.onOpen?.());
      expect(fakes.catalogLists).toHaveLength(1);
      expect(fakes.guide.refresh).not.toHaveBeenCalled();

      await act(async () => fakes.streams[0]?.onOpen?.());
      expect(fakes.catalogLists).toHaveLength(2);
      expect(fakes.guide.refresh).toHaveBeenCalledWith();
      await act(async () => root.unmount());
    });

    it("stays open only while the app is in the foreground, and never opens twice", async () => {
      const { root } = await mountClient();
      expect(fakes.streams.map(({ closed }) => closed)).toEqual([false]);

      await sendAppState("background");
      expect(fakes.streams.map(({ closed }) => closed)).toEqual([true]);

      await sendAppState("active");
      await sendAppState("active");
      expect(fakes.streams.map(({ closed }) => closed)).toEqual([true, false]);

      await act(async () => root.unmount());
      expect(fakes.streams.map(({ closed }) => closed)).toEqual([true, true]);
      expect(appListeners.size).toBe(0);
    });
  });

  describe("the app's foreground and background", () => {
    it("releases playback and cancels the catalog load in the background, then reloads and resumes on return", async () => {
      fakes.catalogHeld = true;
      const { root } = await mountClient();
      expect(fakes.catalogLists).toHaveLength(1);
      expect(fakes.catalogLists[0]?.aborted).toBe(false);

      await sendAppState("background");
      expect(fakes.catalogLists[0]?.aborted).toBe(true);
      expect(fakes.transport.suspend).toHaveBeenCalledTimes(1);
      expect(fakes.transport.resume).not.toHaveBeenCalled();

      await sendAppState("active");
      expect(fakes.transport.resume).toHaveBeenCalledTimes(1);
      // The return asks the server for the catalog again before it retunes anything.
      expect(fakes.catalogLists).toHaveLength(2);
      await act(async () => root.unmount());
    });

    it("loads the server's version and my channels even when the catalog fails to load", async () => {
      vi.stubGlobal(
        "fetch",
        vi.fn(async () => new Response(JSON.stringify({ dirty: false, version: "1.2.3" }), { status: 200 })),
      );
      fakes.catalogFails = true;
      const { client, root } = await mountClient();

      expect(client().catalogState.error).toBe("catalog down");
      expect(fakes.lifetimes).toContain("myChannels:use");
      expect(client().serverVersion).toBe("1.2.3");
      await act(async () => root.unmount());
    });

    it("loads all three at once, so a slow catalog does not hold the others back", async () => {
      vi.stubGlobal(
        "fetch",
        vi.fn(async () => new Response(JSON.stringify({ dirty: false, version: "1.2.3" }), { status: 200 })),
      );
      fakes.catalogHeld = true;
      const { client, root } = await mountClient();

      expect(fakes.catalogLists).toHaveLength(1);
      expect(fakes.lifetimes).toContain("myChannels:use");
      expect(client().serverVersion).toBe("1.2.3");
      await act(async () => root.unmount());
    });

    it("stops listening, and disposes what it built, when the shell goes away", async () => {
      const { root } = await mountClient();
      expect(appListeners.size).toBeGreaterThan(0);

      await act(async () => root.unmount());

      expect(appListeners.size).toBe(0);
      expect(fakes.guide.dispose).toHaveBeenCalledTimes(1);
      expect(fakes.transport.dispose).toHaveBeenCalledTimes(1);
    });
  });

  describe("when the host changes an option but not the credential", () => {
    // Replays the controllers' lifetimes: any use of one after it was disposed (and before a new one
    // was made) is a disposed controller still being handed out.
    const usesAfterDispose = (name: string) => {
      let live = false;
      const dead: number[] = [];
      fakes.lifetimes.forEach((entry, index) => {
        if (!entry.startsWith(`${name}:`)) return;
        const step = entry.slice(name.length + 1);
        if (step === "create") live = true;
        else if (step === "dispose") live = false;
        else if (!live) dead.push(index);
      });
      return dead;
    };

    it("keeps handing out a guide and my channels that are still alive", async () => {
      const { client, rerender, root } = await mountClient(undefined, {
        load: () => Promise.resolve(undefined),
        save: () => Promise.resolve(),
      });

      // A new store rebuilds the player; the guide and my channels read only the paired request.
      await rerender({
        lastChannel: { load: () => Promise.resolve(undefined), save: () => Promise.resolve() },
      });
      await act(async () => fakes.streams.at(-1)?.handlers.onChannel());
      await act(async () => {
        await client().guide.refresh();
        await client().myChannels.refresh();
      });

      expect(usesAfterDispose("guide")).toEqual([]);
      expect(usesAfterDispose("myChannels")).toEqual([]);
      await act(async () => root.unmount());
    });
  });

  describe("when the transport is rebuilt under a live diagnostics reporter", () => {
    it("keeps the reporter and subscribes it to the new transport", async () => {
      const { rerender, root } = await mountClient({ platform: "ios" });
      const previous = fakes.transport;

      // New marks build a new transport; the reporter reads only the paired request.
      fakes.transport = { ...previous, subscribe: vi.fn(() => () => undefined) } as typeof previous;
      await rerender({ marks: { enabled: true } as never });

      expect(fakes.reporterDisposed).not.toHaveBeenCalled();
      expect(fakes.playback.dispose).not.toHaveBeenCalled();
      expect(fakes.transport.subscribe).toHaveBeenCalledWith(fakes.playback.transportEvent);
      await act(async () => root.unmount());
      expect(fakes.reporterDisposed).toHaveBeenCalledTimes(1);
    });
  });

  describe("diagnostics", () => {
    it("sends batches through the paired, authenticated request", async () => {
      const { root } = await mountClient({ platform: "ios" });
      expect(fakes.batchSenders).toEqual([fakes.requests[0]]);
      await act(async () => root.unmount());
    });

    it("reports a player error, every transport event and each channel change, then disposes with the shell", async () => {
      const { client, root, settle } = await mountClient({ platform: "ios" });
      expect(fakes.playback.channelChanged).toHaveBeenLastCalledWith("ten");

      await settle("first-frame");
      expect(fakes.playback.transportEvent).toHaveBeenCalledWith(
        expect.objectContaining({ type: "first-frame" }),
      );

      await act(async () => {
        fakes.transport.emit({
          attemptId: client().controller.getSnapshot().attemptId as number,
          error: "decoder died",
          type: "error",
        });
      });
      expect(fakes.playback.playerError).toHaveBeenCalledWith(
        expect.objectContaining({ channelId: "ten", error: "decoder died" }),
      );

      await act(async () => client().controller.tuneChannel("twenty"));
      expect(fakes.playback.channelChanged).toHaveBeenLastCalledWith("twenty");

      await act(async () => root.unmount());
      expect(fakes.playback.dispose).toHaveBeenCalledTimes(1);
      expect(fakes.reporterDisposed).toHaveBeenCalledTimes(1);
    });

    it("reports nothing when the app names no identity", async () => {
      const { root, settle } = await mountClient();
      await settle("first-frame");
      expect(fakes.playback.transportEvent).not.toHaveBeenCalled();
      expect(fakes.batchSenders).toEqual([]);
      await act(async () => root.unmount());
    });
  });

  describe("the phone's options", () => {
    it("open on the Guide with no picture: a loaded catalog tunes nothing until the viewer picks", async () => {
      const { client, root } = await mountClient(undefined, undefined, phoneClientOptions);

      expect(client().snapshot.catalog.map(({ id }) => id)).toEqual(["ten", "twenty"]);
      expect(client().snapshot.channel).toBeUndefined();
      expect(fakes.transport.replace).not.toHaveBeenCalled();

      await act(async () => client().controller.tuneChannel("twenty"));
      expect(client().snapshot.channel?.id).toBe("twenty");
      await act(async () => root.unmount());
    });

    it("load the next 2 hours of the Guide, where the TV loads the server's default window", async () => {
      const phone = await mountClient(undefined, undefined, phoneClientOptions);
      const at = new Date("2026-11-09T20:00:00Z");
      fakes.guideOptions[0]?.resolveWindow?.(at);
      expect(fakes.guideWindow).toHaveBeenCalledWith({
        at,
        dayOffset: 0,
        hourShift: 0,
        startHour: null,
        windowMinutes: 120,
      });
      await act(async () => phone.root.unmount());

      const tv = await mountClient();
      expect(fakes.guideOptions[1]?.resolveWindow).toBeUndefined();
      await act(async () => tv.root.unmount());
    });
  });

  describe("retrying a failed playback", () => {
    const retryWith = (error?: string) => {
      const refresh = vi.fn();
      const retry = vi.fn();
      retryPairedClient({
        catalogState: { error, loading: false },
        controller: { retry } as never,
        refresh,
      });
      return { refresh, retry };
    };

    it("loads the catalog again when it never loaded", () => {
      const { refresh, retry } = retryWith("Couldn't load channels.");
      expect(refresh).toHaveBeenCalledTimes(1);
      expect(retry).not.toHaveBeenCalled();
    });

    it("re-tunes the channel when the catalog is fine", () => {
      const { refresh, retry } = retryWith();
      expect(retry).toHaveBeenCalledTimes(1);
      expect(refresh).not.toHaveBeenCalled();
    });
  });
});
