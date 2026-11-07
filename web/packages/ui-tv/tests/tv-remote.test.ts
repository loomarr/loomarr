import { describe, expect, it } from "vitest";

import { createTvRemote, type TvRemoteOutcome } from "../index";

// A clock the test advances by hand: one pending timer at most, as the remote keeps.
const manualClock = () => {
  let nowMs = 1_000;
  const timers = new Map<number, { atMs: number; run: () => void }>();
  let nextId = 0;
  return {
    advance(ms: number) {
      nowMs += ms;
      for (const [id, timer] of [...timers]) {
        if (timer.atMs > nowMs) continue;
        timers.delete(id);
        timer.run();
      }
    },
    clearTimeout(handle: unknown) {
      timers.delete(handle as number);
    },
    now: () => nowMs,
    pending: () => timers.size,
    setTimeout(run: () => void, delayMs: number) {
      nextId += 1;
      timers.set(nextId, { atMs: nowMs + delayMs, run });
      return nextId;
    },
  };
};

const remoteWithLog = (numberEntryMs?: number) => {
  const clock = manualClock();
  const outcomes: Array<{ key: string } & TvRemoteOutcome> = [];
  const remote = createTvRemote({
    clock,
    numberEntryMs,
    onEvent: (event, outcome) => outcomes.push({ key: event.key, ...outcome }),
  });
  return { clock, outcomes, remote };
};

describe("createTvRemote", () => {
  it("tunes a typed number once its wait runs out", () => {
    const { clock, outcomes, remote } = remoteWithLog(2_000);
    remote.nativeKey("1", 1, true);
    remote.nativeKey("2", 1, true);
    expect(remote.getSnapshot().numberEntry?.digits).toBe("12");

    clock.advance(1_999);
    expect(outcomes.at(-1)?.intent).toBeUndefined();
    clock.advance(1);

    expect(outcomes.at(-1)).toEqual({
      handled: true,
      intent: { digits: "12", kind: "tune-number" },
      key: "timeout",
    });
    expect(remote.getSnapshot().numberEntry).toBeUndefined();
    expect(clock.pending()).toBe(0);
  });

  it("restarts the wait on every digit", () => {
    const { clock, outcomes, remote } = remoteWithLog(1_200);
    remote.nativeKey("4", 1, true);
    clock.advance(1_000);
    remote.nativeKey("2", 1, true);
    clock.advance(1_000);
    expect(outcomes.some(({ intent }) => intent?.kind === "tune-number")).toBe(false);
    expect(clock.pending()).toBe(1);
    clock.advance(200);
    expect(outcomes.at(-1)?.intent).toEqual({ digits: "42", kind: "tune-number" });
  });

  it("drops a half-typed number when Watching is left", () => {
    const { clock, outcomes, remote } = remoteWithLog();
    const seen: unknown[] = [];
    remote.subscribe(() => seen.push(remote.getSnapshot()));
    remote.nativeKey("7", 1, true);
    remote.leaveWatching();

    expect(remote.getSnapshot().numberEntry).toBeUndefined();
    expect(clock.pending()).toBe(0);
    expect(seen).toHaveLength(2);
    clock.advance(60_000);
    expect(outcomes.map(({ key }) => key)).toEqual(["digit"]);
  });

  it("ignores keys off Watching and counts a press once, on release", () => {
    const { outcomes, remote } = remoteWithLog();
    remote.nativeKey("up", 1, false);
    remote.nativeKey("up", 0, true);
    remote.nativeKey("select", 1, true);
    expect(outcomes).toEqual([]);

    remote.nativeKey("up", 1, true);
    expect(outcomes).toEqual([{ handled: true, intent: { direction: 1, kind: "step" }, key: "up" }]);
  });

  it("does not take a long press letting go for OK, on any screen", () => {
    const { clock, outcomes, remote } = remoteWithLog();
    remote.nativeKey("longSelect", 1, false);
    clock.advance(700);
    expect(remote.consumeLongSelectRelease()).toBe(true);
    // The stamp is spent: the next row press is a real one.
    expect(remote.consumeLongSelectRelease()).toBe(false);

    remote.nativeKey("longSelect", 1, true);
    remote.select();
    expect(outcomes.map(({ intent }) => intent)).toEqual([undefined]);
    remote.select();
    expect(outcomes.at(-1)).toEqual({ handled: true, intent: { kind: "open-guide" }, key: "select" });
  });

  it("lets a real OK through once the long-press window has passed", () => {
    const { clock, outcomes, remote } = remoteWithLog();
    remote.nativeKey("longSelect", 1, false);
    clock.advance(1_501);
    remote.select();
    expect(outcomes.at(-1)?.intent).toEqual({ kind: "open-guide" });
  });

  it("applies a changed wait to the next digit", () => {
    const { clock, outcomes, remote } = remoteWithLog(1_200);
    remote.setNumberEntryMs(5_000);
    remote.nativeKey("3", 1, true);
    clock.advance(4_999);
    expect(outcomes.at(-1)?.intent).toBeUndefined();
    clock.advance(1);
    expect(outcomes.at(-1)?.intent).toEqual({ digits: "3", kind: "tune-number" });
  });

  it("cancels its timer on dispose", () => {
    const { clock, remote } = remoteWithLog();
    remote.nativeKey("9", 1, true);
    remote.dispose();
    expect(clock.pending()).toBe(0);
  });
});
