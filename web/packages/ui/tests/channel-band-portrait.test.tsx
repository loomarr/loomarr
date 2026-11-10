// @vitest-environment jsdom

import { LoomarrProvider } from "@loomarr/design-system";
import type { ReactNode } from "react";
import { act } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, describe, expect, it, vi } from "vitest";

import { PlayerChrome, type PlayerChromeProps } from "../index";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

// The portrait band (#3408): a narrower block, a two-line title, the progress line on its own row, and
// a next time that never wraps. The desktop and landscape band keep their structure.

type El = {
  children: ArrayLike<El>;
  parentElement: El | null;
  querySelector(s: string): El | null;
  textContent: string | null;
};
type Host = Parameters<typeof createRoot>[0] &
  El & { remove(): void; querySelectorAll(s: string): ArrayLike<El> };

const doc = (
  globalThis as unknown as {
    document: { body: { append(n: unknown): void }; createElement(t: string): Host };
  }
).document;
const computed = (el: El) =>
  (globalThis as unknown as { getComputedStyle(n: El): Record<string, string> }).getComputedStyle(el);

const mounted: { host: Host; unmount: () => void }[] = [];
const mount = (node: ReactNode) => {
  const host = doc.createElement("div");
  doc.body.append(host);
  const root = createRoot(host);
  act(() => root.render(<LoomarrProvider theme="dark">{node}</LoomarrProvider>));
  mounted.push({ host, unmount: () => act(() => root.unmount()) });
  return host;
};
afterEach(() => {
  for (const { host, unmount } of mounted.splice(0)) {
    unmount();
    host.remove();
  }
});

const band = {
  channelName: "Saturday Morning Cartoon Club",
  channelNumber: "101",
  clockMs: 1,
  line: "S01E02 “The Upside-Down Garden”",
  next: { name: "A long-running quiz show", startMs: 4 },
  progress: 0.4,
  startMs: 0,
  stopMs: 10,
  title: "Moth & Moonbeam Adventures Club",
};

const keys = {
  density: "touch" as const,
  moreOpen: false,
  muted: false,
  onChannelDown: vi.fn(),
  onChannelUp: vi.fn(),
  onMore: vi.fn(),
  onToggleMute: vi.fn(),
  onTogglePlay: vi.fn(),
  playing: true,
  pointer: false,
  volume: 0.8,
};

const chrome = (layout: PlayerChromeProps["layout"], density: "pointer" | "touch") => (
  <PlayerChrome
    band={band}
    density={density}
    formatTime={(ms) => `t${ms}`}
    keys={{ ...keys, density }}
    layout={layout}
    live={{ lagSeconds: 0, mode: "live" }}
    onGoLive={vi.fn()}
    top={{
      audioOpen: false,
      density,
      fullscreen: false,
      onAudio: vi.fn(),
      onFullscreen: vi.fn(),
      onLeave: vi.fn(),
      pointer: false,
    }}
    visible
  />
);

const byId = (host: Host, id: string) => host.querySelector(`[data-testid="${id}"]`) as El;
const block = (host: Host) =>
  Array.from(host.querySelectorAll("div")).find((el) => el.textContent === "CH101") as El;

describe("the portrait band", () => {
  it("narrows the channel block to 56px, where the landscape phone keeps 72px", () => {
    expect(computed(block(mount(chrome("portrait", "touch")))).width).toBe("56px");
    expect(computed(block(mount(chrome("overlay", "touch")))).width).toBe("72px");
  });

  it("lets the title take two lines, and the episode line one", () => {
    const portrait = mount(chrome("portrait", "touch"));
    const clamp = (el: El) => computed(el).webkitLineClamp;
    expect(clamp(byId(portrait, "band-title"))).toBe("2");
    // The landscape band keeps one line.
    const landscape = mount(chrome("overlay", "touch"));
    expect(clamp(byId(landscape, "band-title"))).not.toBe("2");
  });

  it("gives the progress line a row of its own, with the clocks on the row under it", () => {
    const host = mount(chrome("portrait", "touch"));
    const line = byId(host, "band-progress");
    const lineRow = line.parentElement as El;
    // Nothing shares the line's row: no clock squeezes it.
    expect(lineRow.children).toHaveLength(1);
    expect(lineRow.textContent).toBe("");
    // The clocks sit together on the next row, start at one end and end at the other.
    const clocks = (lineRow.parentElement as El).children[1] as El;
    expect(clocks.children).toHaveLength(2);
    expect(clocks.children[0]?.textContent).toBe("t0");
    expect(clocks.children[1]?.textContent).toBe("t10");
  });

  it("keeps the next time on one line and lets the title take the rest", () => {
    const time = byId(mount(chrome("portrait", "touch")), "band-next-time");
    expect(time.textContent).toBe("t4");
    const style = computed(time);
    expect(style.whiteSpace).toBe("nowrap");
    expect(style.flexShrink).toBe("0");
  });

  it("gives the Now and Next chips a column that fits them, so a chip never touches its text", () => {
    // The touch NEXT chip is 67px wide: in the 56px column it ran into the next time.
    const column = (host: Host) => computed(byId(host, "band-chip-column")).width;
    expect(column(mount(chrome("portrait", "touch")))).toBe("68px");
    expect(column(mount(chrome("overlay", "pointer")))).toBe("56px");
  });

  it("leaves the desktop band as it was: the progress line between its two clocks in one row", () => {
    const host = mount(chrome("overlay", "pointer"));
    const row = byId(host, "band-progress").parentElement as El;
    expect(row.children).toHaveLength(3);
    expect(row.children[0]?.textContent).toBe("t0");
    expect(row.children[2]?.textContent).toBe("t10");
    expect(computed(block(host)).width).toBe("144px");
  });
});
