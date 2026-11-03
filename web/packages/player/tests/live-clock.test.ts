import { createLiveClock, LIVE_DVR_HORIZON_SECONDS, STALL_REJOIN_LAG_SECONDS } from "@loomarr/player";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const START = new Date("2026-09-21T20:00:00Z");

describe("live clock", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(START);
  });
  afterEach(() => vi.useRealTimers());

  describe("position", () => {
    it("starts live with the tune time as the viewer's time", () => {
      const clock = createLiveClock();
      clock.begin();
      expect(clock.state()).toEqual({
        lagSeconds: 0,
        mode: "live",
        noticeRevision: 0,
        viewerTimeMs: START.getTime(),
      });
    });

    it("follows the live edge on the wall clock when the stream reports no position", () => {
      const clock = createLiveClock();
      clock.begin();
      vi.advanceTimersByTime(90_000);
      expect(clock.state().viewerTimeMs).toBe(START.getTime() + 90_000);
    });

    it("reads the displayed frame's programme time and lags the wall clock by the difference", () => {
      const clock = createLiveClock();
      clock.begin();
      clock.pause({ programTimeMs: START.getTime() - 12_000 });
      expect(clock.state()).toMatchObject({
        lagSeconds: 12,
        mode: "paused",
        viewerTimeMs: START.getTime() - 12_000,
      });
    });

    it("derives the displayed time from the offset behind live when no programme time is known", () => {
      const clock = createLiveClock();
      clock.begin();
      clock.pause({ offsetFromLiveSeconds: 20 });
      expect(clock.state().viewerTimeMs).toBe(START.getTime() - 20_000);
    });

    it("never reports a negative offset as the viewer being ahead of live", () => {
      const clock = createLiveClock();
      clock.begin();
      clock.pause({ offsetFromLiveSeconds: -3 });
      expect(clock.state().viewerTimeMs).toBe(START.getTime());
    });
  });

  describe("skew", () => {
    it("measures the wall clock against the server's, not the device's", () => {
      const clock = createLiveClock();
      const serverAhead = START.getTime() + 5 * 60_000;
      clock.begin(serverAhead);
      expect(clock.now()).toBe(serverAhead);
      vi.advanceTimersByTime(10_000);
      expect(clock.now()).toBe(serverAhead + 10_000);
    });

    it("reports no lag for a frame at the server's live edge on a device whose clock is wrong", () => {
      const clock = createLiveClock();
      const serverBehind = START.getTime() - 2 * 3_600_000;
      clock.begin(serverBehind);
      clock.pause({ programTimeMs: serverBehind });
      expect(clock.state().lagSeconds).toBe(0);
      clock.resume();
      vi.advanceTimersByTime(30_000);
      expect(clock.state({ programTimeMs: serverBehind }).lagSeconds).toBe(30);
    });

    it("ignores a server time that is not a finite number", () => {
      const clock = createLiveClock();
      clock.begin(Number.NaN);
      expect(clock.now()).toBe(START.getTime());
    });

    it("drops the previous attempt's skew when the next attempt has no server time", () => {
      const clock = createLiveClock();
      clock.begin(START.getTime() + 60_000);
      clock.begin();
      expect(clock.now()).toBe(START.getTime());
    });
  });

  describe("pausing", () => {
    it("stays paused however long the viewer waits, never jumping back to live on its own", () => {
      const clock = createLiveClock();
      clock.begin();
      clock.pause({ programTimeMs: START.getTime() });
      vi.advanceTimersByTime((LIVE_DVR_HORIZON_SECONDS + 600) * 1_000);
      expect(clock.state()).toMatchObject({ mode: "paused", noticeRevision: 0 });
      expect(clock.state().viewerTimeMs).toBe(START.getTime());
      // The paused frame's lag keeps counting up, so the chrome can show how far behind it is.
      expect(clock.state().lagSeconds).toBe(LIVE_DVR_HORIZON_SECONDS + 600);
    });

    it("ignores position samples while paused: the frame on screen is frozen", () => {
      const clock = createLiveClock();
      clock.begin();
      clock.pause({ programTimeMs: START.getTime() - 5_000 });
      expect(clock.state({ programTimeMs: START.getTime() }).viewerTimeMs).toBe(START.getTime() - 5_000);
    });

    it("resumes behind live from the paused point inside the horizon", () => {
      const clock = createLiveClock();
      clock.begin();
      clock.pause({ programTimeMs: START.getTime() - 12_000 });
      vi.advanceTimersByTime(30_000);

      expect(clock.resume()).toBe("behind");
      expect(clock.state()).toEqual({
        lagSeconds: 42,
        mode: "behind",
        noticeRevision: 0,
        viewerTimeMs: START.getTime() - 12_000,
      });
    });

    it("holds the lag steady while behind live and the stream reports no position", () => {
      const clock = createLiveClock();
      clock.begin();
      clock.pause({ programTimeMs: START.getTime() - 10_000 });
      vi.advanceTimersByTime(20_000);
      clock.resume();
      vi.advanceTimersByTime(60_000);
      expect(clock.state().lagSeconds).toBe(30);
    });

    it("returns to live once, with a notice, when the paused point has left the DVR window", () => {
      const clock = createLiveClock();
      clock.begin();
      clock.pause({ programTimeMs: START.getTime() });
      vi.advanceTimersByTime(LIVE_DVR_HORIZON_SECONDS * 1_000);

      expect(clock.resume()).toBe("expired");
      expect(clock.state()).toMatchObject({ lagSeconds: 0, mode: "live", noticeRevision: 1 });
    });

    it("returns to live with a notice when the stream no longer holds the paused point, inside the horizon", () => {
      const clock = createLiveClock();
      clock.begin();
      clock.pause({ programTimeMs: START.getTime() });
      vi.advanceTimersByTime(60_000);

      expect(clock.resume({ pointAvailable: false })).toBe("expired");
      expect(clock.state()).toMatchObject({ lagSeconds: 0, mode: "live", noticeRevision: 1 });
    });

    it("expires a position the stream lost while the viewer was playing behind live", () => {
      const clock = createLiveClock();
      clock.begin();
      clock.pause({ programTimeMs: START.getTime() });
      clock.resume();
      clock.expire();
      expect(clock.state()).toMatchObject({ lagSeconds: 0, mode: "live", noticeRevision: 1 });
    });

    it("resets the notice for the next attempt", () => {
      const clock = createLiveClock();
      clock.begin();
      clock.pause({ programTimeMs: START.getTime() });
      vi.advanceTimersByTime(LIVE_DVR_HORIZON_SECONDS * 1_000);
      clock.resume();
      clock.begin();
      expect(clock.state().noticeRevision).toBe(0);
    });

    it("goes live explicitly from a paused or behind position", () => {
      const clock = createLiveClock();
      clock.begin();
      clock.pause({ programTimeMs: START.getTime() - 40_000 });
      clock.goLive();
      expect(clock.state()).toMatchObject({ lagSeconds: 0, mode: "live", viewerTimeMs: START.getTime() });
    });

    it("reports the mode it is in", () => {
      const clock = createLiveClock();
      clock.begin();
      expect(clock.mode()).toBe("live");
      clock.pause();
      expect(clock.mode()).toBe("paused");
      clock.resume();
      expect(clock.mode()).toBe("behind");
      clock.goLive();
      expect(clock.mode()).toBe("live");
    });
  });

  describe("stall rejoin", () => {
    const stall = (seconds: number) => {
      const clock = createLiveClock();
      clock.begin();
      const frozenAt = clock.now();
      clock.beginStall();
      vi.advanceTimersByTime(seconds * 1_000);
      clock.endStall("resumed");
      // The stream resumes where it froze: it trails the wall clock by the length of the stall.
      return { clock, rejoin: clock.shouldRejoin({ programTimeMs: frozenAt }) };
    };

    it("rejoins the live edge when a stall left a live viewer more than 30 s behind", () => {
      expect(stall(STALL_REJOIN_LAG_SECONDS + 1).rejoin).toBe(true);
    });

    it("keeps the position after a stall that left the viewer within 30 s of live", () => {
      expect(stall(STALL_REJOIN_LAG_SECONDS).rejoin).toBe(false);
      expect(stall(5).rejoin).toBe(false);
    });

    it("decides once, when the stream first reports where it is", () => {
      const clock = createLiveClock();
      clock.begin();
      clock.beginStall();
      vi.advanceTimersByTime(45_000);
      clock.endStall("resumed");
      expect(clock.shouldRejoin({})).toBe(false); // position not known yet: keep waiting
      expect(clock.shouldRejoin({ programTimeMs: START.getTime() })).toBe(true);
      expect(clock.shouldRejoin({ programTimeMs: START.getTime() })).toBe(false);
    });

    it("measures the lag from the offset behind live when there is no programme time", () => {
      const clock = createLiveClock();
      clock.begin();
      clock.beginStall();
      vi.advanceTimersByTime(45_000);
      clock.endStall("resumed");
      expect(clock.shouldRejoin({ offsetFromLiveSeconds: 45 })).toBe(true);
    });

    it("does not rejoin when the stall ended any way but the stream resuming", () => {
      const clock = createLiveClock();
      clock.begin();
      clock.beginStall();
      vi.advanceTimersByTime(45_000);
      clock.endStall("error");
      expect(clock.shouldRejoin({ programTimeMs: START.getTime() })).toBe(false);
    });

    it("leaves a viewer who paused where they chose to be, even through a stall", () => {
      const clock = createLiveClock();
      clock.begin();
      clock.beginStall();
      vi.advanceTimersByTime(45_000);
      clock.endStall("resumed");
      clock.pause({ programTimeMs: START.getTime() });
      clock.resume();
      expect(clock.shouldRejoin({ programTimeMs: START.getTime() })).toBe(false);
    });

    it("does not rejoin a viewer who was already behind live when the stall began", () => {
      const clock = createLiveClock();
      clock.begin();
      clock.pause({ programTimeMs: START.getTime() });
      clock.resume();
      clock.beginStall();
      vi.advanceTimersByTime(45_000);
      clock.endStall("resumed");
      expect(clock.shouldRejoin({ programTimeMs: START.getTime() })).toBe(false);
    });

    it("measures a stall against the server's clock", () => {
      const clock = createLiveClock();
      const server = START.getTime() + 3_600_000;
      clock.begin(server);
      clock.beginStall();
      vi.advanceTimersByTime(45_000);
      clock.endStall("resumed");
      expect(clock.shouldRejoin({ programTimeMs: server })).toBe(true);
    });
  });
});
