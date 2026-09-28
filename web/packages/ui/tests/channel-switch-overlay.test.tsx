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

// This package compiles without the DOM lib, so type just the jsdom surface these tests use.
type DomElement = {
  getAttribute: (name: string) => string | null;
  innerHTML: string;
  parentElement: DomElement;
  querySelector: (selector: string) => DomElement | null;
  remove: () => void;
  textContent: string | null;
};

const mount = () => {
  const container = (
    globalThis as unknown as {
      document: { createElement: (tagName: string) => Parameters<typeof createRoot>[0] & DomElement };
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

const overlay = (container: DomElement) => container.querySelector('[aria-label^="Tuning in to channel"]');

describe("TV channel switch overlay (B4, #1627)", () => {
  it("shows the channel line and TUNING IN over snow and the channel's still the moment a switch starts", () => {
    const { container, root } = mount();
    act(() => root.render(surface(tuning)));

    const shown = overlay(container);
    expect(shown?.getAttribute("aria-label")).toBe("Tuning in to channel 7, Science Fiction");
    expect(shown?.textContent).toContain("CH 7");
    expect(shown?.textContent).toContain("Science Fiction");
    expect(shown?.textContent).toContain("TUNING IN");
    // The surf card is dropped (maintainer, 2026-09-27): its programme no longer shows here.
    expect(shown?.textContent).not.toContain("The Current Frontier");
    // The snow is drawn, not filtered: FeTurbulence renders nothing on native.
    expect(shown?.innerHTML).toContain("loomarr-snow-");
    expect(shown?.innerHTML).not.toContain("feTurbulence");
    expect(container.innerHTML).toContain(encodeURIComponent("still/seven").replaceAll("%2F", "/"));
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

  it("falls back to the readout over snow alone when there is no still", () => {
    const { container, root } = mount();
    act(() => root.render(surface({ ...tuning, stillUri: undefined })));

    expect(overlay(container)?.textContent).toContain("Science Fiction");
    expect(container.innerHTML).not.toContain("still/seven");
    expect(container.querySelector("img")).toBeNull();
    act(() => root.unmount());
  });

  it("drops a still that fails to load, keeping the readout, so a broken image never shows", () => {
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

  it("centres the loading spinner (its own align-self must not undo the container's centring)", () => {
    const dom = globalThis as unknown as {
      document: { body: { appendChild: (node: unknown) => void } };
      getComputedStyle: (node: DomElement) => { alignSelf: string };
    };
    const { container, root } = mount();
    dom.document.body.appendChild(container);
    act(() =>
      root.render(
        surface({ catalog: [], recentChannelIds: [], status: "empty" }, { loading: true, schedule: {} }),
      ),
    );
    const spinner = container.querySelector('[aria-label="Loading channels"]');
    if (!spinner) throw new Error("no loading spinner rendered");
    // A spinner that aligns itself to the start sits at the screen's left edge unless a
    // shrink-wrapping, centred wrapper carries it.
    expect(dom.getComputedStyle(spinner).alignSelf).toBe("flex-start");
    expect(dom.getComputedStyle(spinner.parentElement).alignSelf).toBe("center");
    // Unmount: a tree left mounted keeps its spinner animating after jsdom tears down, and the
    // next frame throws "window is not defined" as an unhandled error that fails the run.
    act(() => root.unmount());
    container.remove();
  });

  it("is never drawn on touch density (the web Watch page adopts it separately)", () => {
    expect(renderToStaticMarkup(surface(tuning, { density: "touch" }))).not.toContain("Tuning in to channel");
    expect(renderToStaticMarkup(surface(tuning))).toContain("Tuning in to channel");
  });
});
