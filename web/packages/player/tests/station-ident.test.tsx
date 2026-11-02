// @vitest-environment jsdom

import type { PlayerSnapshot } from "@loomarr/player";
import { act, createElement } from "react";
import { createRoot } from "react-dom/client";
import { describe, expect, it, vi } from "vitest";

vi.mock("expo-crypto", () => ({ randomUUID: vi.fn() }));
vi.mock("expo-video", () => ({ createVideoPlayer: vi.fn(), VideoView: vi.fn() }));

const { useStationIdent } = await import("@loomarr/player/native");

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

type Tune = Pick<PlayerSnapshot, "attemptId" | "status">;

/** Renders the hook over a snapshot the test moves on, reading the card's state after each step. */
const probe = (first: Tune) => {
  const seen: boolean[] = [];
  const Probe = ({ tune }: { tune: Tune }) => {
    seen.push(useStationIdent(tune));
    return null;
  };
  const container = (
    globalThis as unknown as {
      document: { createElement: (tag: string) => Parameters<typeof createRoot>[0] };
    }
  ).document.createElement("div");
  const root = createRoot(container);
  const show = (tune: Tune) => act(() => root.render(createElement(Probe, { tune })));
  show(first);
  return { root, shown: () => seen.at(-1), show };
};

describe("the station-ID card (§9.1)", () => {
  it("shows once as each tune settles, for five seconds", () => {
    vi.useFakeTimers();
    const card = probe({ attemptId: 1, status: "tuning" });
    expect(card.shown()).toBe(false);
    card.show({ attemptId: 1, status: "playing" });
    expect(card.shown()).toBe(true);
    act(() => vi.advanceTimersByTime(4_999));
    expect(card.shown()).toBe(true);
    act(() => vi.advanceTimersByTime(1));
    expect(card.shown()).toBe(false);
    act(() => card.root.unmount());
    vi.useRealTimers();
  });

  it("does not come back when the same tune resumes from pause", () => {
    vi.useFakeTimers();
    const card = probe({ attemptId: 1, status: "playing" });
    act(() => vi.advanceTimersByTime(5_000));
    card.show({ attemptId: 1, status: "paused" });
    card.show({ attemptId: 1, status: "playing" });
    expect(card.shown()).toBe(false);
    act(() => card.root.unmount());
    vi.useRealTimers();
  });

  it("drops while the next tune starts, and shows afresh when it settles", () => {
    vi.useFakeTimers();
    const card = probe({ attemptId: 1, status: "playing" });
    card.show({ attemptId: 2, status: "tuning" });
    expect(card.shown()).toBe(false);
    card.show({ attemptId: 2, status: "playing" });
    expect(card.shown()).toBe(true);
    act(() => card.root.unmount());
    vi.useRealTimers();
  });
});
