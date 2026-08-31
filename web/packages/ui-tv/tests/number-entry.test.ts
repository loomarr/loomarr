import { createTvNumberEntryController, type TvNumberEntryTimer } from "@loomarr/ui-tv";
import { describe, expect, it, vi } from "vitest";

const timerHarness = () => {
  let callback: (() => void) | undefined;
  const timer: TvNumberEntryTimer = {
    cancel: vi.fn(() => {
      callback = undefined;
    }),
    schedule: vi.fn((next) => {
      callback = next;
      return Symbol("number-entry");
    }),
  };
  return { fire: () => callback?.(), timer };
};

describe("TV number entry", () => {
  it("accepts only remote digits, keeps the last three, and commits after the bounded delay", () => {
    const onCommit = vi.fn();
    const { fire, timer } = timerHarness();
    const entry = createTvNumberEntryController({ onCommit, timer });

    expect(entry.pushEvent("up")).toBe(false);
    expect(entry.pushEvent("1")).toBe(true);
    expect(entry.pushEvent("2")).toBe(true);
    expect(entry.pushEvent("3")).toBe(true);
    expect(entry.pushEvent("4")).toBe(true);
    expect(entry.getSnapshot()).toEqual({ digits: "234" });
    expect(timer.schedule).toHaveBeenLastCalledWith(expect.any(Function), 1_200);

    fire();
    expect(onCommit).toHaveBeenCalledWith("234");
    expect(entry.getSnapshot()).toEqual({ digits: "" });
  });

  it("commits immediately for OK and cancels without tuning", () => {
    const onCommit = vi.fn();
    const { timer } = timerHarness();
    const entry = createTvNumberEntryController({ onCommit, timer });

    entry.pushEvent("2");
    entry.commit();
    expect(onCommit).toHaveBeenCalledWith("2");

    entry.pushEvent("9");
    entry.cancel();
    expect(entry.getSnapshot().digits).toBe("");
    expect(onCommit).toHaveBeenCalledOnce();
  });
});
