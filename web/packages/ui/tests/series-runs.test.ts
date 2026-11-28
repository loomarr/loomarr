import { describe, expect, it } from "vitest";

import { groupSeriesRuns, type StripAiring } from "../index";

const airing = (start: number, stop: number, fields: Partial<StripAiring> = {}): StripAiring => ({
  kind: "program",
  scheduleBlockId: `b${start}`,
  startMs: start,
  stopMs: stop,
  title: `t${start}`,
  ...fields,
});
const episode = (start: number, stop: number, series: string, number: number) =>
  airing(start, stop, { episode: number, season: 1, series, title: `Part ${number}` });

const shape = (airings: StripAiring[]) =>
  groupSeriesRuns(airings).map((item) =>
    item.type === "break"
      ? `break ${item.startMs}-${item.stopMs}`
      : `${item.episodes} ${item.startMs}-${item.stopMs}`,
  );

describe("groupSeriesRuns", () => {
  it("makes adjacent airings of one series a run with its episode count", () => {
    expect(shape([episode(0, 10, "A", 1), episode(10, 20, "A", 2), episode(20, 30, "B", 1)])).toEqual([
      "2 0-20",
      "1 20-30",
    ]);
  });

  it("carries a run across a break and keeps the break as its own item", () => {
    expect(
      shape([episode(0, 10, "A", 1), airing(10, 12, { kind: "filler" }), episode(12, 20, "A", 2)]),
    ).toEqual(["2 0-20", "break 10-12"]);
  });

  it("counts an episode split by a break once", () => {
    expect(
      shape([episode(0, 10, "A", 1), airing(10, 12, { kind: "filler" }), episode(12, 20, "A", 1)]),
    ).toEqual(["1 0-20", "break 10-12"]);
  });

  it("ends a run at another programme, and never joins films or pending slots", () => {
    expect(
      shape([
        episode(0, 10, "A", 1),
        airing(10, 20),
        episode(20, 30, "A", 2),
        airing(30, 40),
        airing(40, 50),
        airing(50, 60, { kind: "pending", series: "C" }),
        airing(60, 70, { kind: "pending", series: "C" }),
      ]),
    ).toEqual(["1 0-10", "1 10-20", "1 20-30", "1 30-40", "1 40-50", "1 50-60", "1 60-70"]);
  });

  it("orders by start time and treats dead time as a break", () => {
    expect(shape([airing(20, 30), airing(0, 10), airing(10, 20, { kind: "flex" })])).toEqual([
      "1 0-10",
      "break 10-20",
      "1 20-30",
    ]);
  });
});
