import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { DrawerChannel } from "../channel-drawer";
import { useWatchShortcuts } from "./use-watch-shortcuts";

const channels: DrawerChannel[] = [
  { id: "ch-2", number: 7, name: "Saturday Cartoons", now: "A cartoon", minutesLeft: 11, blocks: [] },
];

type Props = Parameters<typeof useWatchShortcuts>[0];

describe("useWatchShortcuts", () => {
  afterEach(() => vi.useRealTimers());

  const setup = (overrides: Partial<Props> = {}) => {
    const onTune = vi.fn();
    const initialProps: Props = { channels, onTune, viewerPaused: false, ...overrides };
    const hook = renderHook((p: Props) => useWatchShortcuts(p), { initialProps });
    const press = (key: string) => {
      let handled = false;
      act(() => {
        handled = hook.result.current.onShortcut(key);
      });
      return handled;
    };
    return { hook, onTune, press, initialProps };
  };

  it("toggles Playback details on I, and opens Channels on G", () => {
    const onDetails = vi.fn();
    const { hook, press } = setup({ onDetails });
    expect(press("i")).toBe(true);
    expect(press("I")).toBe(true);
    expect(onDetails).toHaveBeenCalledTimes(2);
    expect(press("g")).toBe(true);
    expect(hook.result.current.sheet).toBe("channels");
    expect(press("Escape")).toBe(true);
    expect(hook.result.current.sheet).toBeUndefined();
  });

  it("opens Audio on A, goes to the last channel on L", () => {
    const onLast = vi.fn();
    const { hook, press } = setup({ onLast });
    expect(press("a")).toBe(true);
    expect(hook.result.current.sheet).toBe("audio");
    expect(press("A")).toBe(true);
    expect(hook.result.current.sheet).toBeUndefined();
    expect(press("l")).toBe(true);
    expect(onLast).toHaveBeenCalledTimes(1);
  });

  it("steps the subtitles on C, and leaves C to the player while there is nothing to step", () => {
    const onNextSubtitles = vi.fn();
    expect(setup({ onNextSubtitles }).press("c")).toBe(true);
    expect(onNextSubtitles).toHaveBeenCalledTimes(1);
    expect(setup({ channels: undefined }).press("C")).toBe(false);
  });

  it("toggles Playback details as a sheet on a phone, where there is no panel", () => {
    const { hook, press } = setup({ detailsAsSheet: true });
    expect(press("i")).toBe(true);
    expect(hook.result.current.sheet).toBe("details");
    expect(press("I")).toBe(true);
    expect(hook.result.current.sheet).toBeUndefined();
  });

  it("closes the open sheet on Escape, and leaves only when nothing is open", () => {
    const onLeave = vi.fn();
    const { hook, press } = setup({ onLeave });
    press("g");
    expect(press("Escape")).toBe(true);
    expect(hook.result.current.sheet).toBeUndefined();
    expect(onLeave).not.toHaveBeenCalled();
    expect(press("Escape")).toBe(true);
    expect(onLeave).toHaveBeenCalledTimes(1);
  });

  it("leaves keys it has nothing for to the player", () => {
    const { press } = setup({ channels: undefined });
    expect(press("i")).toBe(false);
    expect(press("g")).toBe(false);
    expect(press("7")).toBe(false);
    expect(press("l")).toBe(false);
    expect(press("Escape")).toBe(false);
  });

  it("tunes a typed channel number at once on Enter", () => {
    const { onTune, press } = setup();
    expect(press("7")).toBe(true);
    expect(press("Enter")).toBe(true);
    expect(onTune).toHaveBeenCalledWith("ch-2");
  });

  it("closes Info by itself while the channel plays, but not while paused", () => {
    vi.useFakeTimers();
    const { hook, initialProps } = setup();
    const open = () => act(() => hook.result.current.toggleSheet("info"));
    open();
    act(() => vi.advanceTimersByTime(7_999));
    expect(hook.result.current.sheet).toBe("info");
    act(() => vi.advanceTimersByTime(1));
    expect(hook.result.current.sheet).toBeUndefined();

    hook.rerender({ ...initialProps, viewerPaused: true });
    open();
    act(() => vi.advanceTimersByTime(60_000));
    expect(hook.result.current.sheet).toBe("info");
  });

  it("hands focus back to what opened the sheet", () => {
    const opener = document.createElement("button");
    document.body.append(opener);
    opener.focus();
    const { hook } = setup();
    act(() => hook.result.current.toggleSheet("more"));
    document.body.focus();
    act(() => hook.result.current.closeSheet());
    expect(opener).toHaveFocus();
    opener.remove();
  });
});
