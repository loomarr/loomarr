// @vitest-environment jsdom

import { LoomarrProvider } from "@loomarr/design-system";
import { act } from "react";
import { createRoot } from "react-dom/client";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import { type SurfChannelData, WatchingPanel, type WatchingPanelProps } from "../index";

(
  globalThis as typeof globalThis & {
    IS_REACT_ACT_ENVIRONMENT: boolean;
  }
).IS_REACT_ACT_ENVIRONMENT = true;

interface TestElement {
  click: () => void;
  getAttribute: (name: string) => string | null;
  textContent: string | null;
}
interface TestContainer {
  querySelectorAll: (selector: string) => ArrayLike<TestElement>;
}
const testDocument = (
  globalThis as unknown as { document: { body: { appendChild: (node: unknown) => void } } }
).document;
const createTestContainer = () => {
  const container = (
    globalThis as unknown as {
      document: { createElement: (tagName: string) => Parameters<typeof createRoot>[0] & TestContainer };
    }
  ).document.createElement("div");
  testDocument.body.appendChild(container);
  return container;
};

const favourite: SurfChannelData = {
  channelLogoState: "missing",
  channelName: "Nature Documentaries",
  channelNumber: "21",
  id: "twenty-one",
  now: {
    artworkState: "missing",
    remainingLabel: "24m left",
    seriesTitle: "A nature series",
    timeLabel: "8:00 PM–9:00 PM",
    title: "A nature series · Caves",
  },
};

const handlers = () => ({
  onChannelDown: vi.fn(),
  onChannelUp: vi.fn(),
  onGoLive: vi.fn(),
  onPause: vi.fn(),
  onPlay: vi.fn(),
  onPrevious: vi.fn(),
  onTune: vi.fn(),
});

const props = (overrides: Partial<WatchingPanelProps> = {}): WatchingPanelProps => ({
  ...handlers(),
  canPrevious: true,
  canSurf: true,
  channel: { name: "Late Night Science Fiction", number: "7" },
  clockLabel: "8:26 PM",
  density: "touch",
  favourites: [favourite],
  live: { lagSeconds: 0, mode: "live" },
  schedule: {
    next: { timeLabel: "9:04 PM", title: "A mystery series · Home" },
    now: {
      episodeLabel: "S2E4",
      facts: ["1996", "TV-14"],
      progressPercent: 43,
      remainingLabel: "34m left",
      seriesTitle: "An anthology series",
      timeLabel: "8:00 PM–9:00 PM",
      title: "An anthology series · Expanding Human",
    },
  },
  ...overrides,
});

const markup = (overrides: Partial<WatchingPanelProps> = {}) =>
  renderToStaticMarkup(
    <LoomarrProvider>
      <WatchingPanel {...props(overrides)} />
    </LoomarrProvider>,
  );

const mount = (panelProps: WatchingPanelProps) => {
  const container = createTestContainer();
  const root = createRoot(container);
  act(() =>
    root.render(
      <LoomarrProvider>
        <WatchingPanel {...panelProps} />
      </LoomarrProvider>,
    ),
  );
  const button = (name: string | RegExp) => {
    const found = Array.from(container.querySelectorAll("[role=button]")).find((el) =>
      typeof name === "string"
        ? el.textContent === name || el.getAttribute("aria-label") === name
        : name.test(el.getAttribute("aria-label") ?? el.textContent ?? ""),
    );
    if (!found) throw new Error(`no button ${String(name)}`);
    return found;
  };
  return { button, unmount: () => act(() => root.unmount()) };
};

describe("WatchingPanel", () => {
  it("reads what's on as the mock writes it: Series “Episode”, then episode · year · time", () => {
    const html = markup();
    expect(html).toContain("An anthology series");
    expect(html).toContain(" “Expanding Human”");
    expect(html).not.toContain("An anthology series · Expanding Human");
    expect(html).toContain("S2E4 · 1996 · 8:00 PM–9:00 PM");
    expect(html).not.toContain("TV-14");
    expect(html).toContain("8:26 PM");
    expect(html).toContain("34m left");
    expect(html).toContain("Live");
  });

  it("carries the four controls under the picture, then what's next and the favourites", () => {
    const html = markup();
    for (const label of ["Previous", "Channel −", "Pause", "Channel +"]) expect(html).toContain(label);
    expect(html).toContain("Next 9:04 PM · ");
    expect(html).toContain("FAVORITES · 1");
    expect(html).toContain("A nature series “Caves”");
    expect(html).toContain("24m left");
  });

  it("leaves the favourites out when the viewer has none", () => {
    expect(markup({ favourites: [] })).not.toContain("FAVORITES");
  });

  it("presses through to the tuner and the player", () => {
    const panelProps = props();
    const { button, unmount } = mount(panelProps);
    act(() => button("Previous").click());
    act(() => button("Channel −").click());
    act(() => button("Pause").click());
    act(() => button("Channel +").click());
    act(() => button(/^Watch 21 Nature Documentaries$/).click());
    expect(panelProps.onPrevious).toHaveBeenCalledOnce();
    expect(panelProps.onChannelDown).toHaveBeenCalledOnce();
    expect(panelProps.onPause).toHaveBeenCalledOnce();
    expect(panelProps.onChannelUp).toHaveBeenCalledOnce();
    expect(panelProps.onTune).toHaveBeenCalledWith("twenty-one");
    unmount();
  });

  it("offers Play and Go Live once paused behind the live edge", () => {
    const panelProps = props({ live: { lagSeconds: 83, mode: "paused" } });
    expect(markup({ live: panelProps.live })).toContain("Paused · 1:23 behind");
    const { button, unmount } = mount(panelProps);
    act(() => button("Play").click());
    act(() => button("Go Live").click());
    expect(panelProps.onPlay).toHaveBeenCalledOnce();
    expect(panelProps.onGoLive).toHaveBeenCalledOnce();
    unmount();
  });

  it("disables Previous with nothing to go back to, and surfing with one channel", () => {
    const panelProps = props({ canPrevious: false, canSurf: false });
    const { button, unmount } = mount(panelProps);
    for (const name of ["Previous", "Channel −", "Channel +"]) {
      expect(button(name).getAttribute("aria-disabled")).toBe("true");
      act(() => button(name).click());
    }
    expect(panelProps.onPrevious).not.toHaveBeenCalled();
    expect(panelProps.onChannelDown).not.toHaveBeenCalled();
    expect(panelProps.onChannelUp).not.toHaveBeenCalled();
    unmount();
  });
});
