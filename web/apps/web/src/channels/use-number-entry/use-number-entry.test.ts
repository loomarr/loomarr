import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useNumberEntry } from "./use-number-entry";

const channels = [
  { id: "seven", number: 7 },
  { id: "twelve", number: 12 },
  { id: "hundred", number: 100 },
];

describe("useNumberEntry", () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  const setup = (list = channels) => {
    const onTune = vi.fn();
    const hook = renderHook(() => useNumberEntry({ channels: list, onTune }));
    return { onTune, hook };
  };

  it("offers back any key that isn't a digit", () => {
    const { hook } = setup();
    expect(hook.result.current.press("g")).toBe(false);
    expect(hook.result.current.digits).toBeUndefined();
  });

  it("tunes a short number once the entry window runs out", () => {
    const { onTune, hook } = setup();
    act(() => void hook.result.current.press("7"));
    expect(hook.result.current.digits).toBe("7");
    act(() => vi.advanceTimersByTime(1_199));
    expect(onTune).not.toHaveBeenCalled();
    act(() => vi.advanceTimersByTime(1));
    expect(onTune).toHaveBeenCalledWith("seven");
    expect(hook.result.current.digits).toBeUndefined();
  });

  it("tunes at once on Enter", () => {
    const { onTune, hook } = setup();
    act(() => void hook.result.current.press("1"));
    act(() => void hook.result.current.press("2"));
    act(() => void hook.result.current.commit());
    expect(onTune).toHaveBeenCalledWith("twelve");
  });

  it("tunes shortly after the last digit a channel number can have", () => {
    const { onTune, hook } = setup();
    expect(hook.result.current.width).toBe(3);
    for (const d of "100") act(() => void hook.result.current.press(d));
    act(() => vi.advanceTimersByTime(350));
    expect(onTune).toHaveBeenCalledWith("hundred");
  });

  it("names a number no channel has, then clears", () => {
    const { onTune, hook } = setup();
    act(() => void hook.result.current.press("9"));
    act(() => void hook.result.current.commit());
    expect(onTune).not.toHaveBeenCalled();
    expect(hook.result.current).toMatchObject({ digits: "9", missed: true });
    act(() => vi.advanceTimersByTime(900));
    expect(hook.result.current).toMatchObject({ digits: undefined, missed: false });
  });

  it("has nothing to commit with no entry open", () => {
    const { onTune, hook } = setup();
    expect(hook.result.current.commit()).toBe(false);
    expect(onTune).not.toHaveBeenCalled();
  });
});
