import { createNeighbourWarmer, type PlayerChannel } from "@loomarr/player";
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
    return done.promise;
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
