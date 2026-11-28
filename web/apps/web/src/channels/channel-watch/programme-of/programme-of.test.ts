import { describe, expect, it } from "vitest";
import { programmeOf } from "./programme-of";

const channel = {
  id: "ch-1",
  minutesLeft: 9,
  name: "Noir",
  number: 1,
  onNow: "From the guide",
  playable: true,
};
const time = (ms: number) => `t${ms}`;

describe("programmeOf", () => {
  it("reads the watched channel's schedule at the playhead", () => {
    const band = {
      channelName: "Noir",
      channelNumber: "1",
      line: "S2 E4",
      next: { name: "Late show", startMs: 2_000_000 },
      rating: "TV-PG",
      stopMs: 1_600_000,
      title: "Night desk",
    };
    expect(programmeOf(channel, { band, id: "ch-1", playheadMs: 1_000_000 }, time)).toEqual({
      episode: "S2 E4",
      kind: "on",
      minutesLeft: 10,
      next: { startsAt: "t2000000", title: "Late show" },
      rating: "TV-PG",
      title: "Night desk",
    });
  });

  it("says when a break's programme is back", () => {
    const band = {
      breakMinutesLeft: 3,
      channelName: "Noir",
      channelNumber: "1",
      inBreak: true,
      title: "Night desk",
    };
    expect(programmeOf(channel, { band, id: "ch-1", playheadMs: 0 }, time)).toEqual({
      backInMinutes: 3,
      kind: "break",
      title: "Night desk",
    });
  });

  it("says what the guide has on for another channel, or the watched one before its schedule is read", () => {
    const band = { channelName: "Noir", channelNumber: "1" };
    expect(programmeOf(channel, { band, id: "ch-1", playheadMs: 0 }, time)).toEqual({
      kind: "on",
      minutesLeft: 9,
      title: "From the guide",
    });
    expect(programmeOf({ ...channel, id: "ch-2", onNow: undefined }, undefined, time)).toEqual({
      kind: "nothing",
    });
  });
});
