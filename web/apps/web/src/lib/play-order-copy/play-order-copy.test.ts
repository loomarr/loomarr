import { describe, expect, it } from "vitest";
import { describePlayOrder, describeSpan } from "./play-order-copy";

const HOUR = 3_600_000;

describe("describeSpan", () => {
  it("reads hours below a day, halves of a day under ten, then whole days and weeks", () => {
    expect(describeSpan(0.4 * HOUR)).toBe("1 hour");
    expect(describeSpan(14 * HOUR)).toBe("14 hours");
    expect(describeSpan(12 * HOUR + 24 * HOUR)).toBe("1½ days");
    expect(describeSpan(83 * HOUR)).toBe("3½ days");
    expect(describeSpan(72 * HOUR)).toBe("3 days");
    expect(describeSpan(24 * HOUR)).toBe("1 day");
    expect(describeSpan(15 * 24 * HOUR)).toBe("15 days");
    expect(describeSpan(60 * 24 * HOUR)).toBe("9 weeks");
  });
});

describe("describePlayOrder", () => {
  const episodes = { episodes: 226, movies: 0, series: 1, runtimeMs: 83 * HOUR };

  it("sizes one pass from the backend's numbers", () => {
    expect(describePlayOrder("sequential", episodes)).toBe(
      "226 episodes, start to finish, then it starts over — about every 3½ days.",
    );
  });

  it("says movies and episodes together, and a single title in the singular", () => {
    expect(describePlayOrder("shuffle", { episodes: 1, movies: 1, series: 1, runtimeMs: 3 * HOUR })).toMatch(
      /^1 episode and 1 movie, in a random mix/,
    );
  });

  it("drops the numbers when there is nothing to count", () => {
    const empty = { episodes: 0, movies: 0, series: 0, runtimeMs: 0 };
    expect(describePlayOrder("sequential", empty)).toBe("Plays the list in order, then starts over.");
    expect(describePlayOrder("sequential")).toBe("Plays the list in order, then starts over.");
  });

  it("says nothing for an order it cannot describe truthfully", () => {
    expect(describePlayOrder("time_slot", episodes)).toBeUndefined();
    expect(describePlayOrder("", episodes)).toBeUndefined();
  });

  it("never claims a new order on every pass, which is true only once the shuffle reseeds", () => {
    for (const mode of ["shuffle", "syndication"]) {
      expect(describePlayOrder(mode, episodes)).not.toMatch(/different|new order|each time/i);
    }
  });
});
