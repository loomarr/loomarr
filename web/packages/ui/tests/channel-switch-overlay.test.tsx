// @vitest-environment jsdom

import { LoomarrProvider } from "@loomarr/design-system";
import type { PlayerSnapshot } from "@loomarr/player";
import { act } from "react";
import { createRoot } from "react-dom/client";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import { WatchingSurface } from "../index";

(
  globalThis as typeof globalThis & {
    IS_REACT_ACT_ENVIRONMENT: boolean;
  }
).IS_REACT_ACT_ENVIRONMENT = true;

const channel = { id: "seven", inAppPlayable: true, name: "Science Fiction", number: 7 };
const tuning: PlayerSnapshot = {
  attemptId: 4,
  catalog: [channel],
  channel,
  recentChannelIds: [],
  status: "tuning",
  stillUri: "https://loomarr.test/v1/playout/still/seven?sig=one",
  tuneReason: "step",
};
const playing: PlayerSnapshot = { ...tuning, status: "playing" };
const schedule = {
  next: { timeLabel: "9:30 PM", title: "Later" },
  now: {
    progressPercent: 40,
    remainingLabel: "12m left",
    timeLabel: "9:00 PM–9:30 PM",
    title: "The Current Frontier",
  },
};

const mount = () => {
  const container = (
    globalThis as unknown as {
      document: { createElement: (tagName: string) => Parameters<typeof createRoot>[0] & HTMLElement };
    }
  ).document.createElement("div");
  return { container, root: createRoot(container) };
};

const surface = (snapshot: PlayerSnapshot, props: Partial<Parameters<typeof WatchingSurface>[0]> = {}) => (
  <LoomarrProvider>
    <WatchingSurface
      density="tv"
      onChannelDown={vi.fn()}
      onChannelUp={vi.fn()}
      onDismissControls={vi.fn()}
      onGoLive={vi.fn()}
      onOpenGuide={vi.fn()}
      onOpenSurf={vi.fn()}
      onPause={vi.fn()}
      onPlay={vi.fn()}
      onPrevious={vi.fn()}
      onRetry={vi.fn()}
      onShowControls={vi.fn()}
      player={<div data-player="one-native-player" />}
      schedule={schedule}
      snapshot={snapshot}
      {...props}
    />
  </LoomarrProvider>
);

const overlay = (container: HTMLElement) => container.querySelector('[aria-label="Channel switch"]');

describe("TV channel switch overlay", () => {
  it("shows the surf card and the channel's still the moment a channel switch starts", () => {
    const { container, root } = mount();
    act(() => root.render(surface(tuning)));

    const shown = overlay(container);
    expect(shown).not.toBeNull();
    // The exact surf-list card: number, name, live dot's programme, time left and progress.
    expect(shown?.textContent).toContain("07");
    expect(shown?.textContent).toContain("Science Fiction");
    expect(shown?.textContent).toContain("The Current Frontier");
    expect(shown?.textContent).toContain("12m left");
    expect(shown?.innerHTML).toContain("The Current Frontier"); // the progress track is labelled with the programme
    expect(container.innerHTML).toContain(encodeURIComponent("still/seven").replace("%2F", "/"));
    act(() => root.unmount());
  });

  it("fades out and unmounts once the first frame is decoded (status playing)", () => {
    vi.useFakeTimers();
    const { container, root } = mount();
    act(() => root.render(surface(tuning)));
    expect(overlay(container)).not.toBeNull();

    act(() => root.render(surface(playing)));
    act(() => vi.advanceTimersByTime(10));
    expect(overlay(container)).not.toBeNull(); // still fading, not popped

    act(() => vi.advanceTimersByTime(400));
    expect(overlay(container)).toBeNull();
    act(() => root.unmount());
    vi.useRealTimers();
  });

  it("falls back to the card on the plain background when there is no still", () => {
    const { container, root } = mount();
    act(() => root.render(surface({ ...tuning, stillUri: undefined })));

    expect(overlay(container)?.textContent).toContain("Science Fiction");
    expect(container.innerHTML).not.toContain("still/seven");
    expect(container.querySelector("img")).toBeNull();
    act(() => root.unmount());
  });

  it("drops a still that fails to load, keeping the card, so a broken image never shows", () => {
    vi.useFakeTimers();
    // The native image loader reports failure through the platform Image's onerror.
    class FailingImage {
      onerror?: () => void;
      set src(_value: string) {
        setTimeout(() => this.onerror?.(), 0);
      }
    }
    vi.stubGlobal("Image", FailingImage);
    const { container, root } = mount();
    act(() => root.render(surface(tuning)));
    expect(container.innerHTML).toContain("still/seven");

    act(() => vi.advanceTimersByTime(5));

    expect(container.innerHTML).not.toContain("still/seven");
    expect(overlay(container)?.textContent).toContain("Science Fiction");
    act(() => root.unmount());
    vi.unstubAllGlobals();
    vi.useRealTimers();
  });

  it("does not cover a recovery retry or the initial catalog tune", () => {
    const { container, root } = mount();
    act(() => root.render(surface({ ...tuning, tuneReason: "retry" })));
    expect(overlay(container)).toBeNull();
    act(() => root.render(surface({ ...tuning, reconnecting: { attempt: 1, maxAttempts: 3 } })));
    expect(overlay(container)).toBeNull();
    act(() => root.render(surface({ ...tuning, tuneReason: "catalog" })));
    expect(overlay(container)).toBeNull();
    act(() => root.unmount());
  });

  it("is never drawn on touch density (the web Watch page adopts it separately)", () => {
    expect(renderToStaticMarkup(surface(tuning, { density: "touch" }))).not.toContain("Channel switch");
  });
});
