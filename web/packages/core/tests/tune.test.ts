import { describe, expect, it } from "vitest";

import { idleTune, type TuneState, tuneFlip, tuneKind, tunePictureReady, tuneView } from "../tune";

const warm = (state: TuneState, at: number) => tuneFlip(state, { at, streaming: true });
const cold = (state: TuneState, at: number) => tuneFlip(state, { at, streaming: false });
const coverAt = (state: TuneState, at: number) => tuneView(state, at).cover;

// The times a cover goes up, watching every millisecond of flips at the given times.
const coversFor = (flips: readonly number[], until: number) => {
  let state = idleTune;
  let covered = false;
  const starts: number[] = [];
  const ends: number[] = [];
  for (let now = 0; now <= until; now += 1) {
    if (flips.includes(now)) state = warm(state, now);
    const next = coverAt(state, now) !== "none";
    if (next && !covered) starts.push(now);
    if (!next && covered) ends.push(now);
    covered = next;
  }
  return { ends, starts };
};

describe("choosing warm or cold", () => {
  it("is changing channel only when the next channel already streams and it is not the first tune", () => {
    expect(tuneKind({ streaming: true })).toBe("changing");
    expect(tuneKind({ streaming: false })).toBe("tuning-in");
    expect(tuneKind({ firstTune: true, streaming: true })).toBe("tuning-in");
  });
});

describe("changing channel", () => {
  it("draws about 150 ms of static, clear by 220 ms, with the bars inside 340 ms", () => {
    const state = warm(idleTune, 1000);
    expect(coverAt(state, 1000)).toBe("burst");
    expect(coverAt(state, 1149)).toBe("burst");
    expect(coverAt(state, 1150)).toBe("fading");
    expect(coverAt(state, 1220)).toBe("none");
    expect(tuneView(state, 1339).bars).toBe("run");
    expect(tuneView(state, 1340)).toMatchObject({ bars: "none", cover: "none", nextAt: null });
  });

  it("says when it next changes, so a player needs no clock of its own", () => {
    const state = warm(idleTune, 0);
    expect(tuneView(state, 0).nextAt).toBe(150);
    expect(tuneView(state, 150).nextAt).toBe(220);
    expect(tuneView(state, 220).nextAt).toBe(340);
  });

  it("ignores the picture being ready: it adds no waiting", () => {
    const state = warm(idleTune, 0);
    expect(tunePictureReady(state, 10)).toBe(state);
  });
});

describe("fast flipping", () => {
  it("holds one wash and uncovers once, 220 ms after the last flip", () => {
    const flips = [0, 100, 200, 300, 400, 500, 600, 700, 800, 900, 1000, 1100];
    expect(coversFor(flips, 2000)).toEqual({ ends: [1320], starts: [0] });
    let state = idleTune;
    for (const at of flips) state = warm(state, at);
    expect(coverAt(state, 1319)).toBe("wash");
  });

  it("never starts more than three covers in a second, whatever the rhythm", () => {
    for (const gap of [151, 221, 230, 250, 300, 333, 335, 400, 500]) {
      const flips = Array.from({ length: 20 }, (_, i) => i * gap);
      const { starts } = coversFor(flips, 20 * gap + 1500);
      for (const [i, start] of starts.entries()) {
        const fourth = starts[i + 3];
        if (fourth !== undefined)
          expect(fourth - start, `flips ${gap} ms apart`).toBeGreaterThanOrEqual(1000);
      }
    }
  });
});

describe("tuning in", () => {
  it("covers for exactly the supplied wait, with grey bars that lock when the picture arrives", () => {
    for (const wait of [80, 900, 14_000]) {
      const waiting = cold(idleTune, 0);
      expect(tuneView(waiting, wait - 1)).toMatchObject({ bars: "waiting", cover: "cold", nextAt: null });
      const ready = tunePictureReady(waiting, wait);
      expect(tuneView(ready, wait)).toMatchObject({ bars: "locked", cover: "none" });
      expect(tuneView(ready, wait + 339).bars).toBe("locked");
      expect(tuneView(ready, wait + 340)).toMatchObject({ bars: "none", cover: "none", nextAt: null });
    }
  });

  it("starts the wait again on a flip made while waiting", () => {
    const again = cold(cold(idleTune, 0), 500);
    expect(tuneView(again, 5000).bars).toBe("waiting");
    expect(coverAt(tunePictureReady(again, 5000), 5000)).toBe("none");
  });

  it("gives way to a warm flip made while waiting", () => {
    const state = warm(cold(idleTune, 0), 500);
    expect(coverAt(state, 719)).toBe("wash");
    expect(coverAt(state, 720)).toBe("none");
  });
});
