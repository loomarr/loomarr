import { describe, expect, it } from "vitest";
import { DEFAULT_APPLICATION_FILTERS } from "../../application-filters";
import { eventParams } from "./event-params";

const NOW = 1_780_000_000_000;

describe("eventParams", () => {
  it("asks for every retained event, newest first, in pages of fifty", () => {
    expect(eventParams(DEFAULT_APPLICATION_FILTERS, undefined, NOW)).toEqual({
      from: 1,
      to: NOW,
      limit: 50,
      order: "newest",
    });
  });

  it("bounds a range window back from now", () => {
    expect(eventParams({ ...DEFAULT_APPLICATION_FILTERS, range: "1h" }, undefined, NOW)).toMatchObject({
      from: NOW - 3_600_000,
      to: NOW,
    });
    expect(eventParams({ ...DEFAULT_APPLICATION_FILTERS, range: "24h" }, undefined, NOW).from).toBe(
      NOW - 86_400_000,
    );
  });

  it("carries the cursor and every set filter, trimmed, and omits the unset ones", () => {
    const params = eventParams(
      {
        ...DEFAULT_APPLICATION_FILTERS,
        order: "oldest",
        level: "warn",
        source: "web",
        subsystem: "  player ",
        text: " failed ",
        requestId: "request-1",
        channelId: "channel-1",
        processRunId: "process-1",
      },
      "page-2",
      NOW,
    );
    expect(params).toEqual({
      from: 1,
      to: NOW,
      limit: 50,
      order: "oldest",
      cursor: "page-2",
      level: "warn",
      source: "web",
      subsystem: "player",
      text: "failed",
      requestId: "request-1",
      channelId: "channel-1",
      processRunId: "process-1",
    });
  });

  it("sends no subsystem or text filter for blank input", () => {
    const params = eventParams(
      { ...DEFAULT_APPLICATION_FILTERS, subsystem: "   ", text: " " },
      undefined,
      NOW,
    );
    expect(params).not.toHaveProperty("subsystem");
    expect(params).not.toHaveProperty("text");
  });
});
