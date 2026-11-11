// @vitest-environment jsdom

import { LoomarrProvider } from "@loomarr/design-system";
import type { ReactNode } from "react";
import { act } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, describe, expect, it, vi } from "vitest";

import { PlayerChrome, type PlayerChromeProps } from "../index";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

// The phone's band (#3408, variant J): a 56px block held either way, a two-line title upright, a next
// time that never wraps, and the toolbar above the picture holding Leave and the utilities.

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
    onLeave={vi.fn()}
    utilities={{
      audioOpen: false,
      density,
      fullscreen: false,
      onAudio: vi.fn(),
      onFullscreen: vi.fn(),
      pointer: false,
    }}
    visible
  />
);

const byId = (host: Host, id: string) => host.querySelector(`[data-testid="${id}"]`) as El;
const block = (host: Host) =>
  Array.from(host.querySelectorAll("div")).find((el) => el.textContent === "CH101") as El;

describe("the portrait band", () => {
  it("draws a phone's channel block 56px wide, held either way, and desktop's 96px (§9.2)", () => {
    expect(computed(block(mount(chrome("portrait", "touch")))).width).toBe("56px");
    expect(computed(block(mount(chrome("overlay", "touch")))).width).toBe("56px");
    expect(computed(block(mount(chrome("overlay", "pointer")))).width).toBe("96px");
  });

  it("steps a three-digit number down a size in a phone's block, so it keeps a margin", () => {
    // The fixture's channel is 101: about 50px wide at the full size, in a 56px block.
    const size = (host: Host) => computed(byId(host, "band-channel-number")).fontSize;
    expect(size(mount(chrome("portrait", "touch")))).toBe("22px");
    expect(size(mount(chrome("overlay", "touch")))).toBe("22px");
    expect(size(mount(chrome("overlay", "pointer")))).toBe("48px");
  });

  it("holds Leave and the utilities in the toolbar above the picture", () => {
    const host = mount(chrome("portrait", "touch"));
    const labels = Array.from(host.querySelectorAll('[data-testid="player-toolbar"] [role="button"]')).map(
      (el) => (el as El & { getAttribute(n: string): string | null }).getAttribute("aria-label"),
    );
    expect(labels).toEqual(["Leave", "Audio", "Fullscreen"]);
  });

  it("lets the title take two lines, and the episode line one", () => {
    const portrait = mount(chrome("portrait", "touch"));
    const clamp = (el: El) => computed(el).webkitLineClamp;
    expect(clamp(byId(portrait, "band-title"))).toBe("2");
    // The landscape band keeps one line.
    const landscape = mount(chrome("overlay", "touch"));
    expect(clamp(byId(landscape, "band-title"))).not.toBe("2");
  });

  it("sets the progress line between its two clocks, as every band does (variant J)", () => {
    const row = byId(mount(chrome("portrait", "touch")), "band-progress").parentElement as El;
    expect(row.children).toHaveLength(3);
    expect(row.children[0]?.textContent).toBe("t0");
    expect(row.children[2]?.textContent).toBe("t10");
  });

  it("keeps the next time on one line and lets the title take the rest", () => {
    const time = byId(mount(chrome("portrait", "touch")), "band-next-time");
    expect(time.textContent).toBe("t4");
    const style = computed(time);
    expect(style.whiteSpace).toBe("nowrap");
    expect(style.flexShrink).toBe("0");
  });

  it("keeps the desktop band's progress line between its two clocks in one row, without a strip", () => {
    const host = mount(chrome("overlay", "pointer"));
    const row = byId(host, "band-progress").parentElement as El;
    expect(row.children).toHaveLength(3);
    expect(row.children[0]?.textContent).toBe("t0");
    expect(row.children[2]?.textContent).toBe("t10");
  });
});
