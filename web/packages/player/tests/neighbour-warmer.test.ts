import { createNeighbourWarmer, type PlayerChannel, warmableAssets } from "@loomarr/player";
import { describe, expect, it, vi } from "vitest";

const catalog: PlayerChannel[] = ["a", "b", "c", "d", "e", "f"].map((id, index) => ({
  id,
  inAppPlayable: true,
  name: id,
  number: index + 1,
}));

const deferred = () => {
  let resolve!: () => void;
  const promise = new Promise<void>((onResolve) => {
    resolve = onResolve;
  });
  return { promise, resolve };
};

const setup = (radius?: number) => {
  const started: { id: string; signal: AbortSignal; done: ReturnType<typeof deferred> }[] = [];
  const warm = vi.fn((channel: PlayerChannel, _profile: unknown, signal: AbortSignal) => {
    const done = deferred();
    started.push({ id: channel.id, signal, done });
    return done.promise.then(() => undefined);
  });
  const warmer = createNeighbourWarmer({ profile: { maxResolution: 2160 }, radius, source: { warm } });
  return { started, warm, warmer };
};

const ids = (started: { id: string }[]) => started.map((entry) => entry.id);

describe("neighbour warmer", () => {
  it("warms the adjacent ring first and the outer ring only once it settles", async () => {
    const { started, warmer } = setup(2);
    warmer.retarget(catalog, "c");
    expect(ids(started).sort()).toEqual(["b", "d"]);

    for (const entry of started.slice()) entry.done.resolve();
    await vi.waitFor(() => expect(ids(started).sort()).toEqual(["a", "b", "d", "e"]));
  });

  it("wraps around the ends of the catalog and never warms the current channel", () => {
    const { started, warmer } = setup(1);
    warmer.retarget(catalog, "a");
    expect(ids(started).sort()).toEqual(["b", "f"]);
  });

  it("honours a radius of one", async () => {
    const { started, warmer } = setup(1);
    warmer.retarget(catalog, "c");
    for (const entry of started.slice()) entry.done.resolve();
    await Promise.resolve();
    expect(ids(started).sort()).toEqual(["b", "d"]);
  });

  it("latest request wins: retargeting aborts every stale warm and drops its outer ring", async () => {
    const { started, warmer } = setup(2);
    warmer.retarget(catalog, "c");
    const stale = started.slice();
    warmer.retarget(catalog, "d");
    expect(stale.every((entry) => entry.signal.aborted)).toBe(true);

    // The stale ring settling late must not start its outer ring.
    for (const entry of stale) entry.done.resolve();
    await Promise.resolve();
    await Promise.resolve();
    expect(ids(started).slice(2).sort()).toEqual(["c", "e"]);
  });

  it("cancel aborts in-flight warms and starts nothing more", async () => {
    const { started, warmer } = setup();
    warmer.retarget(catalog, "c");
    const inflight = started.slice();
    warmer.cancel();
    expect(inflight.every((entry) => entry.signal.aborted)).toBe(true);
    for (const entry of inflight) entry.done.resolve();
    await Promise.resolve();
    expect(started).toHaveLength(2);
  });

  it("treats a rejected warm as a harmless miss", async () => {
    const warm = vi.fn().mockRejectedValue(new Error("503"));
    const warmer = createNeighbourWarmer({ profile: { maxResolution: 2160 }, radius: 2, source: { warm } });
    warmer.retarget(catalog, "c");
    await vi.waitFor(() => expect(warm).toHaveBeenCalledTimes(4));
  });

  it("does nothing when the source cannot warm or the catalog has no neighbours", () => {
    const warmer = createNeighbourWarmer({ profile: { maxResolution: 2160 }, source: {} });
    expect(() => warmer.retarget(catalog, "c")).not.toThrow();
    const { started, warmer: solo } = setup();
    solo.retarget([catalog[0] as PlayerChannel], "a");
    expect(started).toHaveLength(0);
  });
});

describe("neighbour warmer default radius", () => {
  it("warms one channel each side, matching the server's two retained warm sessions", async () => {
    const { started, warmer } = setup();
    warmer.retarget(catalog, "c");
    for (const entry of started.slice()) entry.done.resolve();
    await Promise.resolve();
    await Promise.resolve();
    expect(ids(started).sort()).toEqual(["b", "d"]);
  });
});

describe("neighbour warmer reuse", () => {
  const warmedSource = (id: string, expiresAt?: number) => ({
    expiresAt,
    serverTimeMs: 1_000,
    uri: `https://loomarr.test/${id}.m3u8?sig=warm`,
    warmed: true,
  });

  it("hands back the exact warmed source once, aged so live chrome stays on server time", async () => {
    vi.useFakeTimers();
    try {
      vi.setSystemTime(10_000);
      const warm = vi.fn((channel: PlayerChannel) => Promise.resolve(warmedSource(channel.id, 10_000_000)));
      const warmer = createNeighbourWarmer({ profile: {}, source: { warm } });
      warmer.retarget(catalog, "c");
      await vi.advanceTimersByTimeAsync(0);

      vi.setSystemTime(40_000);
      const taken = warmer.take("d");
      expect(taken?.uri).toBe("https://loomarr.test/d.m3u8?sig=warm");
      expect(taken?.serverTimeMs).toBe(31_000);
      expect(warmer.take("d")).toBeUndefined();
    } finally {
      vi.useRealTimers();
    }
  });

  it("never hands back a source that is about to expire, a non-neighbour, or an unwarmed miss", async () => {
    const now = Date.now();
    const warm = vi.fn((channel: PlayerChannel) =>
      Promise.resolve(
        channel.id === "b" ? { ...warmedSource("b", now + 10_000) } : { ...warmedSource("d"), warmed: false },
      ),
    );
    const warmer = createNeighbourWarmer({ profile: {}, source: { warm } });
    warmer.retarget(catalog, "c");
    await vi.waitFor(() => expect(warm).toHaveBeenCalledTimes(2));
    await Promise.resolve();

    expect(warmer.take("b")).toBeUndefined();
    expect(warmer.take("d")?.uri).toContain("sig=warm");
    expect(warmer.take("f")).toBeUndefined();
  });

  it("drops warmed sources for channels the viewer has moved away from", async () => {
    const warm = vi.fn((channel: PlayerChannel) => Promise.resolve(warmedSource(channel.id)));
    const warmer = createNeighbourWarmer({ profile: {}, source: { warm } });
    warmer.retarget(catalog, "c");
    await vi.waitFor(() => expect(warm).toHaveBeenCalledTimes(2));
    await Promise.resolve();
    warmer.retarget(catalog, "f");
    expect(warmer.take("d")).toBeUndefined();
  });
});

describe("warmableAssets", () => {
  it("selects the newest fragment and the map active across its discontinuity", () => {
    const manifest = `#EXTM3U
#EXT-X-MAP:URI="old/init.mp4?sig=x"
#EXTINF:2,
old/seg.m4s?sig=x
#EXT-X-DISCONTINUITY
#EXT-X-MAP:URI="new/init.mp4?sig=x"
#EXTINF:2,
new/seg.m4s?sig=x
`;
    expect(warmableAssets(manifest)).toEqual(["new/init.mp4?sig=x", "new/seg.m4s?sig=x"]);
  });
});
