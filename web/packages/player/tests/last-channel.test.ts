import {
  createLastChannelStore,
  createPlayerController,
  type PlayerChannel,
  type PlayerSourcePort,
  type PlayerTransport,
} from "@loomarr/player";
import { describe, expect, it, vi } from "vitest";

const channels: PlayerChannel[] = [
  { id: "ten", inAppPlayable: true, name: "Ten", number: 10 },
  { id: "thirty", inAppPlayable: true, name: "Thirty", number: 30 },
];

const memoryStorage = (initial?: string) => {
  const items = new Map<string, string>(initial ? [["loomarr.lastChannelId", initial]] : []);
  return {
    getItem: vi.fn((key: string) => Promise.resolve(items.get(key) ?? null)),
    items,
    setItem: vi.fn((key: string, value: string) => {
      items.set(key, value);
      return Promise.resolve();
    }),
  };
};

const controllerWith = (
  options: Partial<Parameters<typeof createPlayerController>[0]>,
  mint = vi.fn((channel: PlayerChannel) =>
    Promise.resolve({ uri: `https://loomarr.test/${channel.id}.m3u8` }),
  ),
) => {
  const transport: PlayerTransport = {
    dispose: vi.fn(),
    goLive: vi.fn(),
    pause: vi.fn(),
    play: vi.fn(),
    replace: vi.fn().mockResolvedValue(undefined),
    subscribe: vi.fn(() => () => undefined),
  };
  const source: PlayerSourcePort = { mint };
  return { controller: createPlayerController({ profile: {}, source, transport, ...options }), mint };
};

describe("last channel store", () => {
  it("returns what it saved, and nothing before anything is saved", async () => {
    const storage = memoryStorage();
    const store = createLastChannelStore(storage);
    expect(await store.load()).toBeUndefined();
    await store.save("thirty");
    expect(await store.load()).toBe("thirty");
    expect(storage.items.get("loomarr.lastChannelId")).toBe("thirty");
  });

  it("treats a store that can't be read as nothing remembered", async () => {
    const store = createLastChannelStore({
      getItem: () => Promise.reject(new Error("keychain locked")),
      setItem: () => Promise.resolve(),
    });
    expect(await store.load()).toBeUndefined();
  });
});

describe("resuming the last channel", () => {
  it("tunes the remembered channel instead of the first one", async () => {
    const store = createLastChannelStore(memoryStorage("thirty"));
    const { controller, mint } = controllerWith({ restoreChannelId: store.load });
    await controller.reconcile(channels);
    expect(controller.getSnapshot()).toMatchObject({ channel: { id: "thirty" }, tuneReason: "catalog" });
    expect(mint).toHaveBeenCalledTimes(1);
  });

  it.each([
    ["a channel that is no longer in the catalog", "retired"],
    ["nothing remembered", undefined],
  ])("falls back to the first channel with %s", async (_label, remembered) => {
    const store = createLastChannelStore(memoryStorage(remembered));
    const { controller } = controllerWith({ restoreChannelId: store.load });
    await controller.reconcile(channels);
    expect(controller.getSnapshot().channel?.id).toBe("ten");
  });

  it("falls back to the first channel when the remembered one is not playable in the app", async () => {
    const store = createLastChannelStore(memoryStorage("thirty"));
    const { controller } = controllerWith({ restoreChannelId: store.load });
    await controller.reconcile([
      { id: "ten", inAppPlayable: true, name: "Ten", number: 10 },
      { id: "thirty", inAppPlayable: false, name: "Thirty", number: 30 },
    ]);
    expect(controller.getSnapshot().channel?.id).toBe("ten");
  });

  it("tunes the first channel when the store throws", async () => {
    const { controller } = controllerWith({ restoreChannelId: () => Promise.reject(new Error("locked")) });
    await controller.reconcile(channels);
    expect(controller.getSnapshot().channel?.id).toBe("ten");
  });

  it("tunes once when two catalogs arrive while the stored channel is being read", async () => {
    const store = createLastChannelStore(memoryStorage("thirty"));
    const { controller, mint } = controllerWith({ restoreChannelId: store.load });
    await Promise.all([controller.reconcile(channels), controller.reconcile(channels)]);
    expect(controller.getSnapshot().channel?.id).toBe("thirty");
    expect(mint).toHaveBeenCalledTimes(1);
  });

  it("offers an idle controller's Guide and Watching the remembered channel without tuning", async () => {
    const store = createLastChannelStore(memoryStorage("thirty"));
    const { controller, mint } = controllerWith({ initialTune: "none", restoreChannelId: store.load });
    await controller.reconcile(channels);
    expect(controller.getSnapshot()).toMatchObject({ restoredChannelId: "thirty", status: "idle" });
    expect(mint).not.toHaveBeenCalled();
    await controller.tuneChannel("thirty");
    expect(controller.getSnapshot().restoredChannelId).toBeUndefined();
  });

  it("offers nothing on an idle controller when the remembered channel is gone", async () => {
    const store = createLastChannelStore(memoryStorage("retired"));
    const { controller } = controllerWith({ initialTune: "none", restoreChannelId: store.load });
    await controller.reconcile(channels);
    expect(controller.getSnapshot()).toMatchObject({ restoredChannelId: undefined, status: "idle" });
  });
});
