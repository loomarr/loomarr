import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useCopied } from "./use-copied";

const setClipboard = (value: unknown) =>
  Object.defineProperty(navigator, "clipboard", { configurable: true, value });

afterEach(() => {
  vi.useRealTimers();
  setClipboard(undefined);
  Reflect.deleteProperty(document, "execCommand");
});

describe("useCopied", () => {
  it("never says copied when nothing could be written (insecure origin, no fallback)", async () => {
    setClipboard(undefined);
    Object.defineProperty(document, "execCommand", { configurable: true, value: () => false });
    const { result } = renderHook(() => useCopied());

    let written: boolean | undefined;
    await act(async () => {
      written = await result.current.copy("x");
    });

    expect(written).toBe(false);
    expect(result.current.copied).toBeUndefined();
  });

  it("says copied after a fallback copy on an insecure origin, then clears the acknowledgement", async () => {
    setClipboard(undefined);
    Object.defineProperty(document, "execCommand", { configurable: true, value: () => true });
    const { result } = renderHook(() => useCopied());

    vi.useFakeTimers();
    await act(async () => {
      await result.current.copy("x");
    });
    expect(result.current.copied).toBe(true);

    await act(async () => {
      vi.advanceTimersByTime(2000);
    });
    expect(result.current.copied).toBeUndefined();
  });

  it("clears its pending timer on unmount", async () => {
    setClipboard({ writeText: vi.fn().mockResolvedValue(undefined) });
    const { result, unmount } = renderHook(() => useCopied());
    vi.useFakeTimers();
    await act(async () => {
      await result.current.copy("x");
    });

    unmount();

    expect(vi.getTimerCount()).toBe(0);
  });
});
