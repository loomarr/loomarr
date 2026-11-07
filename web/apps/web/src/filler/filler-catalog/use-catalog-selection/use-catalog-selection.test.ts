import type { ClipDTO } from "@loomarr/api";
import { act, renderHook } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { useCatalogSelection } from "./use-catalog-selection";

const clip = (hash: string): ClipDTO => ({ hash, name: hash }) as ClipDTO;

describe("useCatalogSelection", () => {
  it("toggles a clip in and out by hash", () => {
    const { result } = renderHook(() => useCatalogSelection("page-1"));

    act(() => result.current.toggle("a"));
    expect([...result.current.selected]).toEqual(["a"]);

    act(() => result.current.toggle("a"));
    expect(result.current.selected.size).toBe(0);
  });

  it("selects every row shown, and doubles as the un-select once all are picked", () => {
    const rows = [clip("a"), clip("b")];
    const { result } = renderHook(() => useCatalogSelection("page-1"));
    expect(result.current.allSelected(rows)).toBe(false);

    act(() => result.current.selectAll(rows));
    expect([...result.current.selected].sort()).toEqual(["a", "b"]);
    expect(result.current.allSelected(rows)).toBe(true);

    act(() => result.current.selectAll(rows));
    expect(result.current.selected.size).toBe(0);
  });

  it("never reports an empty page as fully selected", () => {
    const { result } = renderHook(() => useCatalogSelection("page-1"));
    expect(result.current.allSelected([])).toBe(false);
  });

  it("drops the selection when the result scope changes, but not on a re-render of the same scope", () => {
    const { result, rerender } = renderHook(({ scope }) => useCatalogSelection(scope), {
      initialProps: { scope: "page-1" },
    });
    act(() => result.current.toggle("a"));

    rerender({ scope: "page-1" });
    expect(result.current.selected.size).toBe(1);

    rerender({ scope: "page-2" });
    expect(result.current.selected.size).toBe(0);
  });
});
