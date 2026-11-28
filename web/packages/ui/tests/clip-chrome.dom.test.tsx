// @vitest-environment jsdom

import { act, cleanup, renderUi } from "@loomarr/test-support";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { ClipChrome, type ClipChromeProps } from "../index";

const mount = (node: ReactNode) => renderUi(node).container;
afterEach(cleanup);

const button = (host: ParentNode, name: string) =>
  Array.from(host.querySelectorAll<HTMLElement>('[role="button"]')).find(
    (el) => el.getAttribute("aria-label") === name,
  );

const props = (overrides: Partial<ClipChromeProps> = {}): ClipChromeProps => ({
  density: "pointer",
  elapsedMs: 0,
  fullscreen: false,
  muted: false,
  onFullscreen: vi.fn(),
  onToggleMute: vi.fn(),
  onTogglePlay: vi.fn(),
  onVolume: vi.fn(),
  playing: false,
  pointer: true,
  shown: true,
  volume: 1,
  ...overrides,
});

describe("ClipChrome", () => {
  // A button permanently called "Play" lies the moment the video plays; a screen reader has no
  // other cue. The same holds for mute and fullscreen, whose names say which way they go.
  it("names each control for what it will do next", () => {
    const idle = mount(<ClipChrome {...props()} />);
    expect(button(idle, "Play")).toBeDefined();
    expect(button(idle, "Mute")).toBeDefined();
    expect(button(idle, "Fullscreen")).toBeDefined();

    const busy = mount(<ClipChrome {...props({ fullscreen: true, muted: true, playing: true })} />);
    expect(button(busy, "Pause")).toBeDefined();
    expect(button(busy, "Unmute")).toBeDefined();
    expect(button(busy, "Exit fullscreen")).toBeDefined();
  });

  it("hands each press to its handler", () => {
    const p = props();
    const host = mount(<ClipChrome {...p} />);

    act(() => button(host, "Play")?.click());
    act(() => button(host, "Mute")?.click());
    act(() => button(host, "Fullscreen")?.click());

    expect(p.onTogglePlay).toHaveBeenCalledOnce();
    expect(p.onToggleMute).toHaveBeenCalledOnce();
    expect(p.onFullscreen).toHaveBeenCalledOnce();
  });

  // A muted player reads as zero on the slider, whatever level it will return to.
  it("reads a muted level as zero on the volume slider", () => {
    const host = mount(<ClipChrome {...props({ muted: true, volume: 0.6 })} />);
    expect(host.querySelector('[aria-label="Volume"]')?.getAttribute("aria-valuenow")).toBe("0");
  });

  // The readout waits for the file's length: "0:00 / 0:00" while metadata loads reads as an
  // empty clip.
  it("shows elapsed over length once the length is known", () => {
    expect(mount(<ClipChrome {...props({ elapsedMs: 5000 })} />).textContent).not.toContain("/");
    expect(mount(<ClipChrome {...props({ durationMs: 95_000, elapsedMs: 5000 })} />).textContent).toContain(
      "00:05 / 01:35",
    );
  });

  // An untitled clip embedded under a heading that already names it draws no empty top band.
  it("draws the top scrim only for a title or a leading control", () => {
    expect(mount(<ClipChrome {...props()} />).querySelector('[data-testid="clip-chrome-top"]')).toBeNull();

    const titled = mount(<ClipChrome {...props({ title: "Tidepool Hour" })} />);
    expect(titled.querySelector('[data-testid="clip-chrome-top"]')?.textContent).toBe("Tidepool Hour");

    const leading = mount(<ClipChrome {...props({ leading: <span>Close</span> })} />);
    expect(leading.querySelector('[data-testid="clip-chrome-top"]')?.textContent).toBe("Close");
  });

  // Hidden chrome stays mounted so it can fade, but it must be out of reach of assistive tech.
  it("hides from assistive tech while the player has hidden it", () => {
    const underHidden = (host: ParentNode, selector: string) =>
      Array.from(host.querySelectorAll('[aria-hidden="true"]')).some((el) =>
        Boolean(el.querySelector(selector)),
      );
    const hiddenTitle = (host: ParentNode) =>
      Array.from(host.querySelectorAll('[aria-hidden="true"]')).some(
        (el) => el.textContent === "Tidepool Hour",
      );

    const hidden = mount(<ClipChrome {...props({ shown: false, title: "Tidepool Hour" })} />);
    expect(hiddenTitle(hidden)).toBe(true);
    expect(underHidden(hidden, '[aria-label="Play"]')).toBe(true);

    const shown = mount(<ClipChrome {...props({ title: "Tidepool Hour" })} />);
    expect(hiddenTitle(shown)).toBe(false);
    expect(underHidden(shown, '[aria-label="Play"]')).toBe(false);
  });
});
