import type { EventView } from "@loomarr/api/models/eventView";
import { describe, expect, it } from "vitest";
import { fallbackMessage, formatTime, levelLabel, levelTone, sourceLabel } from "./event-format";

const event = (name: string): EventView => ({
  id: "event",
  occurredAt: 0,
  receivedAt: 0,
  level: "info",
  source: "server",
  event: name,
  attributes: {},
});

describe("event format", () => {
  it("gives each level a tone and a readable label", () => {
    expect(levelTone("error")).toBe("error");
    expect(levelTone("warn")).toBe("warn");
    expect(levelTone("info")).toBe("ok");
    expect(levelTone("debug")).toBe("off");
    expect(levelLabel("warn")).toBe("Warning");
    expect(levelLabel("error")).toBe("Error");
    expect(levelLabel("debug")).toBe("Debug");
  });

  it("names the source runtime", () => {
    expect(sourceLabel("server")).toBe("Loomarr");
    expect(sourceLabel("android_tv")).toBe("Android TV");
  });

  it("turns a dotted event name into words when there is no message", () => {
    expect(fallbackMessage(event("playout.transition_late"))).toBe("Playout Transition Late");
    expect(fallbackMessage(event("app..ready"))).toBe("App Ready");
  });

  it("formats a timestamp with the date and the seconds", () => {
    const millis = Date.UTC(2026, 10, 14, 12, 34, 56);
    const expected = new Intl.DateTimeFormat(undefined, { dateStyle: "medium", timeStyle: "medium" }).format(
      new Date(millis),
    );
    expect(formatTime(millis)).toBe(expected);
  });
});
