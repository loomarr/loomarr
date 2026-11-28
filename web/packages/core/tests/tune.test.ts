import { describe, expect, it } from "vitest";

import {
  idleTune,
  type TuneState,
  tuneAbandon,
  tuneFlip,
  tuneKind,
  tunePictureReady,
  tuneView,
} from "../tune";

const warm = (state: TuneState, at: number) => tuneFlip(state, { at, streaming: true });
const cold = (state: TuneState, at: number) => tuneFlip(state, { at, streaming: false });
const ready = (state: TuneState, at: number) => tunePictureReady(state, state.flips, at);
const coverAt = (state: TuneState, at: number) => tuneView(state, at).cover;

// The times a cover goes up and comes down, watching every millisecond of warm flips.
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
    expect(ready(state, 10)).toBe(state);
    expect(tuneAbandon(state, state.flips)).toBe(state);
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

  it("shows a flip just after a cover bare, with the bars still running", () => {
    for (const at of [221, 300, 333]) {
      const state = warm(warm(idleTune, 0), at);
      expect(tuneView(state, at), `flip at ${at}`).toMatchObject({
        bars: "run",
        cover: "none",
        nextAt: at + 340,
      });
    }
    expect(coverAt(warm(warm(idleTune, 0), 334), 334)).toBe("burst");
  });

  it("suppresses covers rather than holding one up", () => {
    expect(coversFor([0, 300, 600, 900], 2000)).toEqual({ ends: [220, 820], starts: [0, 600] });
    expect(coversFor([0, 221, 442], 2000)).toEqual({ ends: [220, 662], starts: [0, 442] });
  });

  it("never starts more than three covers in a second, whatever the rhythm", () => {
    for (const gap of [151, 221, 230, 250, 300, 333, 334, 335, 400, 500]) {
      const flips = Array.from({ length: 20 }, (_, i) => i * gap);
      const { starts } = coversFor(flips, 20 * gap + 1500);
      for (const [i, start] of starts.entries()) {
        const fourth = starts[i + 3];
        if (fourth !== undefined)
          expect(fourth - start, `flips ${gap} ms apart`).toBeGreaterThanOrEqual(1000);
      }
    }
  });

  it("never keeps a tune-in covered past its picture, even after rapid flips", () => {
    const state = cold(warm(warm(idleTune, 0), 300), 600);
    expect(coverAt(state, 649)).toBe("cold");
    expect(tuneView(ready(state, 650), 650)).toMatchObject({ bars: "locked", cover: "none" });
  });
});

describe("tuning in", () => {
  it("covers for exactly the supplied wait, with grey bars that lock when the picture arrives", () => {
    for (const wait of [80, 900, 14_000]) {
      const waiting = cold(idleTune, 0);
      expect(tuneView(waiting, wait - 1)).toMatchObject({ bars: "waiting", cover: "cold", nextAt: null });
      const arrived = ready(waiting, wait);
      expect(tuneView(arrived, wait)).toMatchObject({ bars: "locked", cover: "none" });
      expect(tuneView(arrived, wait + 339).bars).toBe("locked");
      expect(tuneView(arrived, wait + 340)).toMatchObject({ bars: "none", cover: "none", nextAt: null });
    }
  });

  it("ignores the picture of a channel already flipped past", () => {
    const a = cold(idleTune, 0);
    const b = cold(a, 500);
    const late = tunePictureReady(b, tuneView(a, 0).flip, 600);
    expect(tuneView(late, 600)).toMatchObject({ bars: "waiting", cover: "cold" });
    expect(coverAt(tunePictureReady(late, tuneView(b, 500).flip, 700), 700)).toBe("none");
  });

  it("can be abandoned without locking the bars", () => {
    const waiting = cold(idleTune, 0);
    expect(tuneAbandon(waiting, waiting.flips - 1)).toBe(waiting);
    const abandoned = tuneAbandon(waiting, waiting.flips);
    expect(tuneView(abandoned, 100)).toMatchObject({ bars: "none", cover: "none", nextAt: null });
    expect(ready(abandoned, 200)).toBe(abandoned);
    expect(coverAt(cold(abandoned, 400), 400)).toBe("cold");
  });

  it("gives way to a warm flip made while waiting", () => {
    const state = warm(cold(idleTune, 0), 500);
    expect(coverAt(state, 719)).toBe("wash");
    expect(coverAt(state, 720)).toBe("none");
  });
});

describe("awkward times", () => {
  it("clears at once for a zero wait, a negative one, or a picture dated before its flip", () => {
    for (const at of [1000, 999, -50]) {
      const arrived = ready(cold(idleTune, 1000), at);
      expect(tuneView(arrived, 1000), `ready at ${at}`).toMatchObject({ bars: "locked", cover: "none" });
      expect(tuneView(arrived, 1340)).toMatchObject({ bars: "none", nextAt: null });
    }
  });

  it("starts afresh on a flip dated before the previous one, and still uncovers", () => {
    for (const previous of [warm(idleTune, 5000), cold(idleTune, 5000)]) {
      const state = warm(previous, 100);
      expect(coverAt(state, 100)).toBe("burst");
      expect(coverAt(state, 320)).toBe("none");
    }
  });
});
