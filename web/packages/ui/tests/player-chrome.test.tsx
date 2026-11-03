// @vitest-environment jsdom

import { LoomarrProvider } from "@loomarr/design-system";
import type { ReactNode } from "react";
import { act } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, describe, expect, it, vi } from "vitest";

import { PlayerChrome, type PlayerChromeProps, PlayerCover, type PlayerTransportProps } from "../index";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

// The package compiles without the DOM lib (it is React Native first), so the test names the little
// of the DOM it touches.
type Node = { click(): void; getAttribute(n: string): string | null; textContent: string | null };
type Host = Parameters<typeof createRoot>[0] & {
  querySelector(s: string): Node | null;
  querySelectorAll(s: string): ArrayLike<Node>;
  remove(): void;
  textContent: string | null;
};
const doc = (
  globalThis as unknown as {
    document: { body: { append(n: unknown): void }; createElement(t: string): Host };
  }
).document;

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

const button = (host: Host, name: string) =>
  Array.from(host.querySelectorAll('[role="button"]')).find(
    (el) => el.getAttribute("aria-label") === name || el.textContent === name,
  );

// Whether the lower third sits under an aria-hidden ancestor (the hidden scrim).
const bandHidden = (host: Host) =>
  Array.from(host.querySelectorAll('[aria-hidden="true"]')).some((el) =>
    el.textContent?.includes("CH 7 · Coast 90s"),
  );

const transport = (overrides: Partial<PlayerTransportProps> = {}): PlayerTransportProps => ({
  density: "pointer",
  live: { lagSeconds: 0, mode: "live" },
  muted: false,
  onGoLive: vi.fn(),
  onLeave: vi.fn(),
  onSettings: vi.fn(),
  onToggleMute: vi.fn(),
  onTogglePlay: vi.fn(),
  onVolume: vi.fn(),
  playing: true,
  pointer: true,
  settingsOpen: false,
  volume: 1,
  ...overrides,
});

const chrome = (overrides: Partial<PlayerChromeProps> = {}) => (
  <PlayerChrome
    band={{
      channelName: "Coast 90s",
      channelNumber: "7",
      programmeLine: "A sitcom · S2 E4",
      timeLabel: "9m in · 13m left",
    }}
    density="pointer"
    formatTime={(ms) => String(ms)}
    transport={transport()}
    visible
    {...overrides}
  />
);

describe("PlayerChrome", () => {
  it("carries the lower third and the transport, each control named", () => {
    const host = mount(chrome());

    expect(host.textContent).toContain("CH 7 · Coast 90s");
    expect(host.textContent).toContain("A sitcom · S2 E4");
    expect(host.textContent).toContain("9m in · 13m left");
    for (const name of ["Pause", "Mute", "Settings", "Leave Watch"])
      expect(button(host, name), name).toBeDefined();
    // Live: the state reads Live, with no Go live to offer.
    expect(host.textContent).toContain("Live");
    expect(button(host, "Go live")).toBeUndefined();
  });

  it("offers Go live when behind, reading how far", () => {
    const onGoLive = vi.fn();
    const host = mount(
      chrome({ transport: transport({ live: { lagSeconds: 134, mode: "paused" }, onGoLive }) }),
    );

    expect(host.textContent).toContain("Paused · 2m 14s behind");
    act(() => button(host, "Go live")?.click());
    expect(onGoLive).toHaveBeenCalledOnce();
  });

  it("draws only the tools the platform has", () => {
    const bare = mount(chrome());
    expect(button(bare, "Channels")).toBeUndefined();
    expect(button(bare, "Fullscreen")).toBeUndefined();

    const full = mount(
      chrome({ transport: transport({ fullscreen: false, onChannels: vi.fn(), onFullscreen: vi.fn() }) }),
    );
    expect(button(full, "Channels")).toBeDefined();
    expect(button(full, "Fullscreen")).toBeDefined();
  });

  it("leaves the picture bare when hidden, out of reach of the pointer and assistive tech", () => {
    expect(bandHidden(mount(chrome()))).toBe(false);
    expect(bandHidden(mount(chrome({ visible: false })))).toBe(true);
  });

  it("puts an open sheet over a dimmed picture, standing the controls back", () => {
    const host = mount(
      chrome({
        sheet: (
          <div aria-label="Settings" role="dialog">
            sheet
          </div>
        ),
      }),
    );

    expect(host.querySelector('[role="dialog"]')?.textContent).toBe("sheet");
    expect(bandHidden(host)).toBe(true);
  });
});

describe("PlayerCover", () => {
  it("is an alert for a stream that stopped, with Retry and Leave", () => {
    const onRetry = vi.fn();
    const onLeave = vi.fn();
    const host = mount(
      <PlayerCover
        channelLabel="CH 7 · Coast 90s"
        density="pointer"
        kind="error"
        onLeave={onLeave}
        onRetry={onRetry}
        title="The stream stopped."
      />,
    );

    expect(host.querySelector('[role="alert"]')?.textContent).toContain("The stream stopped.");
    act(() => button(host, "Retry channel")?.click());
    act(() => button(host, "Leave")?.click());
    expect(onRetry).toHaveBeenCalledOnce();
    expect(onLeave).toHaveBeenCalledOnce();
  });

  it("is a status for an off-air channel, with only the way out", () => {
    const host = mount(
      <PlayerCover
        channelLabel="CH 7 · Coast 90s"
        density="pointer"
        kind="off-air"
        onLeave={vi.fn()}
        title="Off air"
      />,
    );

    expect(host.querySelector('[role="status"]')?.textContent).toContain("Off air");
    expect(button(host, "Retry channel")).toBeUndefined();
    expect(button(host, "Leave")).toBeDefined();
  });
});
