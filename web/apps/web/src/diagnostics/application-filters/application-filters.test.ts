import { describe, expect, it } from "vitest";
import { DEFAULT_APPLICATION_FILTERS, eventSources, sourceNames } from "./application-filters";

describe("application filters", () => {
  it("start by showing every event, newest first", () => {
    expect(DEFAULT_APPLICATION_FILTERS).toEqual({
      range: "all",
      order: "newest",
      level: "all",
      source: "all",
      subsystem: "",
      text: "",
    });
  });

  it("offer every named source", () => {
    expect(eventSources.map((source) => sourceNames[source])).toEqual(Object.values(sourceNames));
    expect(eventSources).toContain("server");
  });
});
