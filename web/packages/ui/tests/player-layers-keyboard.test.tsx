// @vitest-environment jsdom

import { LoomarrProvider } from "@loomarr/design-system";
import type { ReactNode } from "react";
import { act } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, describe, expect, it, vi } from "vitest";

import { PlayerBottomSheet, PlayerDetailsPanel, PlayerMenu, PlayerSheet } from "../index";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

// The player's layers are non-modal dialogs over a picture that keeps playing. Axe passing says
// nothing about the keyboard, so this drives it: where focus lands when a layer opens, that nothing
// traps it, that a key pressed inside reaches the player's frame (which leaves an open dialog's
// arrows alone), and where focus is once the layer closes. The package compiles without the DOM lib,
// so the test names the little of it that it touches.
type El = {
  contains(other: unknown): boolean;
  dispatchEvent(event: unknown): boolean;
  focus(): void;
  getAttribute(name: string): string | null;
  textContent: string | null;
};
type Host = Parameters<typeof createRoot>[0] & {
  contains(other: unknown): boolean;
  querySelector(selector: string): El | null;
  querySelectorAll(selector: string): ArrayLike<El>;
  remove(): void;
};
const dom = globalThis as unknown as {
  document: {
    activeElement: unknown;
    body: { append(node: unknown): void };
    createElement(tagName: string): Host;
  };
  KeyboardEvent: new (type: string, init: { bubbles: boolean; key: string }) => unknown;
};

const mounted: { host: Host; unmount: () => void }[] = [];

// `layer` is mounted beside an opener button, inside a frame that records the keys that reach it.
const open = (layer: ReactNode) => {
  const host = dom.document.createElement("div");
  dom.document.body.append(host);
  const root = createRoot(host);
  const onFrameKey = vi.fn();
  const frame = (shown: boolean) => (
    <LoomarrProvider theme="dark">
      {/* biome-ignore lint/a11y/noStaticElementInteractions: stands in for the player frame's key handler */}
      <div onKeyDown={onFrameKey}>
        <button type="button">Opener</button>
        {shown ? layer : null}
      </div>
    </LoomarrProvider>
  );
  act(() => root.render(frame(true)));
  mounted.push({ host, unmount: () => act(() => root.unmount()) });
  return { close: () => act(() => root.render(frame(false))), host, onFrameKey };
};

afterEach(() => {
  for (const { host, unmount } of mounted.splice(0)) {
    unmount();
    host.remove();
  }
});

const dialog = (host: Host, name: string) =>
  Array.from(host.querySelectorAll('[role="dialog"]')).find((el) => el.getAttribute("aria-label") === name);

const press = (target: El, key: string) =>
  act(() => {
    target.dispatchEvent(new dom.KeyboardEvent("keydown", { bubbles: true, key }));
  });

const layers: { layer: ReactNode; name: string; opensOnClose: boolean }[] = [
  {
    layer: (
      <PlayerSheet
        channel={{ name: "Coast 90s", number: "7" }}
        density="pointer"
        onClose={vi.fn()}
        title="Audio"
      >
        <button type="button">Row</button>
      </PlayerSheet>
    ),
    name: "Audio",
    opensOnClose: true,
  },
  {
    layer: (
      <PlayerBottomSheet onClose={vi.fn()} title="Subtitles">
        <button type="button">Row</button>
      </PlayerBottomSheet>
    ),
    name: "Subtitles",
    opensOnClose: true,
  },
  {
    layer: (
      <PlayerDetailsPanel
        density="pointer"
        onClose={vi.fn()}
        rows={[{ label: "Codec", value: "H.264 High 4.1" }]}
      />
    ),
    name: "Playback details",
    opensOnClose: false,
  },
  {
    layer: (
      <PlayerMenu density="pointer" title="More">
        <button type="button">Row</button>
      </PlayerMenu>
    ),
    name: "More",
    opensOnClose: false,
  },
];

describe.each(layers)("the player's $name layer", ({ layer, name, opensOnClose }) => {
  it("is a named dialog that is not modal", () => {
    const { host } = open(layer);
    const found = dialog(host, name);
    expect(found).toBeDefined();
    expect(found?.getAttribute("aria-modal")).toBeNull();
  });

  it("leaves a key pressed inside it to the frame: nothing swallows Escape or the arrows", () => {
    const { host, onFrameKey } = open(layer);
    const inside = host.querySelectorAll("[role='dialog'] [role='button'], [role='dialog'] button")[0] as El;
    press(inside, "Escape");
    press(inside, "ArrowDown");
    expect(onFrameKey).toHaveBeenCalledTimes(2);
  });

  it("enters focus only where it always did: a sheet's close key takes it, a menu and a panel do not", () => {
    const { host } = open(layer);
    const active = dom.document.activeElement as El;
    const found = dialog(host, name) as El;
    if (opensOnClose) {
      expect(found.contains(active)).toBe(true);
      expect(active.getAttribute("aria-label")).toBe(`Close ${name}`);
    } else {
      expect(found.contains(active)).toBe(false);
    }
  });

  it("leaves nothing behind when it closes, and focus is not stranded in a layer that is gone", () => {
    const { close, host } = open(layer);
    close();
    expect(dialog(host, name)).toBeUndefined();
    const active = dom.document.activeElement;
    expect(active === dom.document.body || host.contains(active)).toBe(true);
  });
});

describe("a pointer layer in the same frame as its opener", () => {
  it("keeps the opener reachable: nothing marks the rest of the page inert or hidden", () => {
    const { host } = open(
      <PlayerMenu density="pointer" title="More">
        <button type="button">Row</button>
      </PlayerMenu>,
    );
    const opener = host.querySelector("button") as El;
    expect(opener.getAttribute("aria-hidden")).toBeNull();
    expect(opener.getAttribute("inert")).toBeNull();
  });
});
