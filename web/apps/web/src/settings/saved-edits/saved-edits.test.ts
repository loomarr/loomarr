import type { settingsPatchResponse } from "@loomarr/api/endpoints/settings";
import { describe, expect, it } from "vitest";
import { everyEditSaved, savedKeysOf } from "./saved-edits";

// The server answers a settings PATCH per key, so the fixture is the per-key result list.
const answered = (results: { key: string; status: string }[]) =>
  ({ status: 200, data: { results } }) as unknown as settingsPatchResponse;

const failed = { status: 500, data: { title: "boom" } } as unknown as settingsPatchResponse;

describe("savedKeysOf", () => {
  it("keeps only the submitted keys the server saved", () => {
    const response = answered([
      { key: "a", status: "saved" },
      { key: "b", status: "invalid" },
      { key: "c", status: "pinned" },
    ]);
    expect(savedKeysOf(response, { a: "1", b: "2", c: "3" })).toEqual(["a"]);
  });

  it("ignores a saved key that was not submitted", () => {
    const response = answered([{ key: "other", status: "saved" }]);
    expect(savedKeysOf(response, { a: "1" })).toEqual([]);
  });

  it("keeps nothing from a failed request", () => {
    expect(savedKeysOf(failed, { a: "1" })).toEqual([]);
  });
});

describe("everyEditSaved", () => {
  it("is true when every submitted key was saved", () => {
    const response = answered([
      { key: "a", status: "saved" },
      { key: "b", status: "saved" },
    ]);
    expect(everyEditSaved(response, { a: "1", b: "2" })).toBe(true);
  });

  it("is false when any submitted key was rejected", () => {
    const response = answered([
      { key: "a", status: "saved" },
      { key: "b", status: "invalid" },
    ]);
    expect(everyEditSaved(response, { a: "1", b: "2" })).toBe(false);
  });

  it("is false when there was nothing to save", () => {
    expect(everyEditSaved(answered([]), {})).toBe(false);
  });

  it("is false for a failed request", () => {
    expect(everyEditSaved(failed, { a: "1" })).toBe(false);
  });
});
