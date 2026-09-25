import { createCatalogRefresher, createPlayerController, type PlayerChannel } from "@loomarr/player";
import { describe, expect, it, vi } from "vitest";

const channels: PlayerChannel[] = [{ id: "simpsons", inAppPlayable: true, name: "Simpsons", number: 7 }];

const deferred = <T>() => {
  let resolve!: (value: T) => void;
  let reject!: (error: unknown) => void;
  const promise = new Promise<T>((onResolve, onReject) => {
    resolve = onResolve;
    reject = onReject;
  });
  return { promise, reject, resolve };
};

const setup = () => {
  const controller = createPlayerController({
    profile: {},
    source: { mint: vi.fn(() => Promise.resolve({ uri: "https://loomarr.test/a.m3u8" })) },
    transport: {
      dispose: vi.fn(),
      goLive: vi.fn(),
      pause: vi.fn(),
      play: vi.fn(),
      replace: vi.fn().mockResolvedValue(undefined),
      subscribe: vi.fn(() => () => undefined),
    },
  });
  const list = vi.fn((_signal: AbortSignal) => Promise.resolve(channels));
  const refresher = createCatalogRefresher({ controller, list });
  return { controller, list, refresher };
};

describe("catalog refresher", () => {
  it("shows the loading state only for the initial load", async () => {
    const { list, refresher } = setup();
    const first = deferred<PlayerChannel[]>();
    list.mockReturnValueOnce(first.promise);
    const pending = refresher.refresh();
    expect(refresher.getState().loading).toBe(true);
    first.resolve(channels);
    await pending;
    expect(refresher.getState()).toEqual({ loading: false });

    const second = deferred<PlayerChannel[]>();
    list.mockReturnValueOnce(second.promise);
    const background = refresher.refresh();
    expect(refresher.getState().loading).toBe(false);
    second.resolve(channels);
    await background;
    expect(refresher.getState().loading).toBe(false);
  });

  it("keeps the player's channel and status through a refresh while playing", async () => {
    const { controller, refresher } = setup();
    await refresher.refresh();
    const before = controller.getSnapshot();
    await refresher.refresh();
    expect(controller.getSnapshot()).toMatchObject({
      channel: { id: before.channel?.id },
      status: before.status,
    });
  });

  it("coalesces a burst of channel events into one in-flight request plus one trailing refresh", async () => {
    const { list, refresher } = setup();
    await refresher.refresh();
    list.mockClear();
    const inflight = deferred<PlayerChannel[]>();
    list.mockReturnValueOnce(inflight.promise);
    const runs = [1, 2, 3, 4, 5].map(() => refresher.refresh());
    expect(list).toHaveBeenCalledTimes(1);
    expect(vi.mocked(list).mock.calls[0]?.[0].aborted).toBe(false);
    inflight.resolve(channels);
    await Promise.all(runs);
    expect(list).toHaveBeenCalledTimes(2);
  });

  it("does not surface a background refresh failure as an error once a catalog is loaded", async () => {
    const { list, refresher } = setup();
    await refresher.refresh();
    list.mockRejectedValueOnce(new Error("502"));
    await expect(refresher.refresh()).rejects.toThrow("502");
    expect(refresher.getState()).toEqual({ loading: false });
  });

  it("surfaces an initial-load failure", async () => {
    const { list, refresher } = setup();
    list.mockRejectedValueOnce(new Error("offline"));
    await expect(refresher.refresh()).rejects.toThrow("offline");
    expect(refresher.getState()).toEqual({ error: "offline", loading: false });
  });
});
