import type { settingsPatchResponse } from "@loomarr/api/endpoints/settings";
import { act, renderHook } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { useSettingsDraft } from "./settings-draft";

const answered = (results: { key: string; status: string }[]) =>
  ({ status: 200, data: { results } }) as unknown as settingsPatchResponse;

describe("useSettingsDraft", () => {
  it("holds the keys that were changed, and only those", () => {
    const { result } = renderHook(() => useSettingsDraft());
    expect(result.current.edits).toEqual({});

    act(() => result.current.setEdit("a", "1"));
    act(() => result.current.setEdit("b", "2"));
    act(() => result.current.setEdit("a", "3"));

    expect(result.current.edits).toEqual({ a: "3", b: "2" });
  });

  it("drops the saved keys and keeps the refused ones", () => {
    const { result } = renderHook(() => useSettingsDraft());
    act(() => result.current.setEdit("a", "1"));
    act(() => result.current.setEdit("b", "2"));

    act(() =>
      result.current.dropSaved(
        answered([
          { key: "a", status: "saved" },
          { key: "b", status: "invalid" },
        ]),
        { a: "1", b: "2" },
      ),
    );

    expect(result.current.edits).toEqual({ b: "2" });
  });

  it("keeps a key typed while the save was in flight", () => {
    const { result } = renderHook(() => useSettingsDraft());
    act(() => result.current.setEdit("a", "1"));
    act(() => result.current.setEdit("late", "x"));

    act(() => result.current.dropSaved(answered([{ key: "a", status: "saved" }]), { a: "1" }));

    expect(result.current.edits).toEqual({ late: "x" });
  });

  it("keeps everything when the request failed", () => {
    const { result } = renderHook(() => useSettingsDraft());
    act(() => result.current.setEdit("a", "1"));

    const failed = { status: 500, data: { title: "boom" } } as unknown as settingsPatchResponse;
    act(() => result.current.dropSaved(failed, { a: "1" }));

    expect(result.current.edits).toEqual({ a: "1" });
  });

  it("clears named keys and discards the lot", () => {
    const { result } = renderHook(() => useSettingsDraft());
    act(() => result.current.setEdit("a", "1"));
    act(() => result.current.setEdit("b", "2"));

    act(() => result.current.clearEdits(["a"]));
    expect(result.current.edits).toEqual({ b: "2" });

    act(() => result.current.resetEdits());
    expect(result.current.edits).toEqual({});
  });
});
