import { describe, expect, it, vi } from "vitest";

vi.mock("expo-video", () => ({
  createVideoPlayer: vi.fn(),
  VideoView: vi.fn(),
}));

const { createNativePlayerLifecycle } = await import("@loomarr/player/native");

const snapshot = {
  catalog: [],
  channel: { id: "seven", inAppPlayable: true, name: "Seven", number: 7 },
  overlayVisible: true,
  recentChannelIds: ["six"],
  status: "paused" as const,
};

describe("native player application lifecycle", () => {
  it("pauses controller state before synchronously releasing the native player", () => {
    const order: string[] = [];
    const lifecycle = createNativePlayerLifecycle({
      controller: {
        getSnapshot: () => snapshot,
        pause: () => order.push("pause"),
        retry: vi.fn(),
      },
      refresh: vi.fn(),
      transport: {
        resume: vi.fn(),
        suspend: () => order.push("suspend"),
      },
    });

    lifecycle.enterBackground();

    expect(order).toEqual(["pause", "suspend"]);
  });

  it("recreates the player and refreshes authority before retuning the remembered channel", async () => {
    const order: string[] = [];
    const lifecycle = createNativePlayerLifecycle({
      controller: {
        getSnapshot: () => snapshot,
        pause: vi.fn(),
        retry: async () => {
          order.push("retry");
        },
      },
      refresh: async () => {
        order.push("refresh");
      },
      transport: {
        resume: () => order.push("resume"),
        suspend: vi.fn(),
      },
    });

    await lifecycle.enterForeground();

    expect(order).toEqual(["resume", "refresh", "retry"]);
  });

  it("does not invent a tune when no remembered channel remains", async () => {
    const retry = vi.fn();
    const lifecycle = createNativePlayerLifecycle({
      controller: {
        getSnapshot: () => ({ ...snapshot, channel: undefined, status: "empty" }),
        pause: vi.fn(),
        retry,
      },
      refresh: vi.fn().mockResolvedValue(undefined),
      transport: { resume: vi.fn(), suspend: vi.fn() },
    });

    await lifecycle.enterForeground();

    expect(retry).not.toHaveBeenCalled();
  });

  it("does not let a stale foreground refresh restart playback after backgrounding", async () => {
    let finishRefresh!: () => void;
    const retry = vi.fn();
    const lifecycle = createNativePlayerLifecycle({
      controller: {
        getSnapshot: () => snapshot,
        pause: vi.fn(),
        retry,
      },
      refresh: () =>
        new Promise<void>((resolve) => {
          finishRefresh = resolve;
        }),
      transport: { resume: vi.fn(), suspend: vi.fn() },
    });

    const foreground = lifecycle.enterForeground();
    lifecycle.enterBackground();
    finishRefresh();
    await foreground;

    expect(retry).not.toHaveBeenCalled();
  });
});
