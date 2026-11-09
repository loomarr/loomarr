import { describe, expect, it } from "vitest";
import { dailyCron, parseDaily } from "./cron-time";

describe("daily cron", () => {
  it("reads the backup default as 3:30", () => {
    expect(parseDaily("0 30 3 * * *")).toEqual({ hour: 3, minute: 30 });
  });

  it("round-trips an hour and a minute", () => {
    expect(parseDaily(dailyCron({ hour: 23, minute: 5 }))).toEqual({ hour: 23, minute: 5 });
  });

  it("does not claim a schedule that is not once a day", () => {
    expect(parseDaily("0 */5 * * * *")).toBeUndefined();
    expect(parseDaily("0 30 3 * * 1")).toBeUndefined();
    expect(parseDaily("0 61 3 * * *")).toBeUndefined();
    expect(parseDaily("0 0 24 * * *")).toBeUndefined();
  });
});
