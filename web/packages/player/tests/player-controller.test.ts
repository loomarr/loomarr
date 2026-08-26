import {
  createPlayerController,
  type PlayerChannel,
  type PlayerSourcePort,
  type PlayerTransport,
  type PlayerTransportEvent,
  playableCatalog,
} from "@loomarr/player";
import { describe, expect, it, vi } from "vitest";

const channels: PlayerChannel[] = [
  { id: "thirty", inAppPlayable: true, name: "Thirty", number: 30 },
  { id: "ten", inAppPlayable: true, name: "Ten", number: 10 },
  { id: "twenty", inAppPlayable: false, name: "Twenty", number: 20 },
];

const deferred = <T>() => {
  let resolve!: (value: T) => void;
  let reject!: (error: unknown) => void;
  const promise = new Promise<T>((onResolve, onReject) => {
    resolve = onResolve;
    reject = onReject;
  });
  return { promise, reject, resolve };
};

const harness = (source?: PlayerSourcePort) => {
  const listeners = new Set<(event: PlayerTransportEvent) => void>();
  const transport: PlayerTransport = {
    dispose: vi.fn(),
    goLive: vi.fn(),
    pause: vi.fn(),
    play: vi.fn(),
    replace: vi.fn().mockResolvedValue(undefined),
    subscribe: vi.fn((listener) => {
      listeners.add(listener);
      return () => listeners.delete(listener);
    }),
  };
  const sourcePort: PlayerSourcePort =
    source ??
    ({
      mint: vi.fn((channel) => Promise.resolve({ uri: `https://loomarr.test/${channel.id}.m3u8` })),
    } satisfies PlayerSourcePort);
  const controller = createPlayerController({
    profile: { maxResolution: 2160 },
    source: sourcePort,
    transport,
  });
  return {
    controller,
    emit: (event: PlayerTransportEvent) => {
      for (const listener of listeners) listener(event);
    },
    source: sourcePort,
    transport,
  };
};

describe("player controller", () => {
  it("uses only server-declared playable channels with stable number ordering", () => {
    expect(playableCatalog(channels).map(({ id }) => id)).toEqual(["ten", "thirty"]);
  });

  it("starts deterministically, wraps, and tunes exact numbers without guessing", async () => {
    const { controller } = harness();
    await controller.reconcile(channels);
    expect(controller.getSnapshot()).toMatchObject({ channel: { id: "ten" }, status: "tuning" });

    await controller.step(-1);
    expect(controller.getSnapshot().channel?.id).toBe("thirty");
    await controller.step(1);
    expect(controller.getSnapshot().channel?.id).toBe("ten");

    await controller.tuneNumber("3");
    expect(controller.getSnapshot().channel?.id).toBe("ten");
    await controller.tuneNumber("030");
    expect(controller.getSnapshot().channel?.id).toBe("thirty");
  });

  it("aborts an older mint and never lets it replace the latest request", async () => {
    const first = deferred<{ uri: string }>();
    const second = deferred<{ uri: string }>();
    const signals: AbortSignal[] = [];
    const source: PlayerSourcePort = {
      mint: vi.fn((_channel, _profile, signal) => {
        signals.push(signal);
        return signals.length === 1 ? first.promise : second.promise;
      }),
    };
    const { controller, transport } = harness(source);

    const initial = controller.reconcile(channels);
    const latest = controller.tuneChannel("thirty");
    expect(signals[0]?.aborted).toBe(true);

    second.resolve({ uri: "https://loomarr.test/thirty.m3u8" });
    await latest;
    first.resolve({ uri: "https://loomarr.test/ten.m3u8" });
    await initial;

    expect(transport.replace).toHaveBeenCalledTimes(1);
    expect(transport.replace).toHaveBeenCalledWith(
      { uri: "https://loomarr.test/thirty.m3u8" },
      expect.objectContaining({ attemptId: 2 }),
    );
  });

  it("reconciles by identity and falls back only when the tuned channel disappears", async () => {
    const { controller, source } = harness();
    await controller.reconcile(channels);
    await controller.tuneChannel("thirty");
    vi.mocked(source.mint).mockClear();

    await controller.reconcile([{ ...channels[0]!, name: "Thirty renamed" }, channels[1]!]);
    expect(controller.getSnapshot().channel).toMatchObject({ id: "thirty", name: "Thirty renamed" });
    expect(source.mint).not.toHaveBeenCalled();

    await controller.reconcile([channels[1]!]);
    expect(controller.getSnapshot()).toMatchObject({ channel: { id: "ten" }, tuneReason: "catalog" });
    expect(source.mint).toHaveBeenCalledTimes(1);
  });

  it("keeps bounded newest-first history and previous toggles through the same tune seam", async () => {
    const many = Array.from({ length: 9 }, (_, index) => ({
      id: `channel-${index + 1}`,
      inAppPlayable: true,
      name: `Channel ${index + 1}`,
      number: index + 1,
    }));
    const { controller } = harness();
    await controller.reconcile(many);
    for (const channel of many.slice(1)) await controller.tuneChannel(channel.id);

    expect(controller.getSnapshot().recentChannelIds).toEqual([
      "channel-8",
      "channel-7",
      "channel-6",
      "channel-5",
      "channel-4",
      "channel-3",
    ]);
    await controller.previous();
    expect(controller.getSnapshot().channel?.id).toBe("channel-8");
    expect(controller.getSnapshot().previousChannelId).toBe("channel-9");
  });

  it("ignores stale transport events and exposes only the current attempt", async () => {
    const { controller, emit } = harness();
    await controller.reconcile(channels);
    const firstAttempt = controller.getSnapshot().attemptId!;
    await controller.tuneChannel("thirty");
    const latestAttempt = controller.getSnapshot().attemptId!;

    emit({ attemptId: firstAttempt, type: "first-frame" });
    expect(controller.getSnapshot().status).toBe("tuning");
    emit({ attemptId: latestAttempt, type: "first-frame" });
    expect(controller.getSnapshot().status).toBe("playing");
    emit({ attemptId: latestAttempt, error: "decoder failed", type: "error" });
    expect(controller.getSnapshot()).toMatchObject({ error: "decoder failed", status: "failed" });
  });

  it("synchronously aborts, pauses, unsubscribes, and releases on dispose", async () => {
    const pending = deferred<{ uri: string }>();
    let signal: AbortSignal | undefined;
    const { controller, emit, transport } = harness({
      mint: vi.fn((_channel, _profile, nextSignal) => {
        signal = nextSignal;
        return pending.promise;
      }),
    });
    const tuning = controller.reconcile(channels);
    const beforeDispose = controller.getSnapshot();

    controller.dispose();
    expect(signal?.aborted).toBe(true);
    expect(transport.pause).toHaveBeenCalledOnce();
    expect(transport.dispose).toHaveBeenCalledOnce();
    emit({ attemptId: beforeDispose.attemptId!, type: "first-frame" });
    expect(controller.getSnapshot()).toBe(beforeDispose);

    pending.resolve({ uri: "https://loomarr.test/late.m3u8" });
    await tuning;
    expect(transport.replace).not.toHaveBeenCalled();
  });
});
