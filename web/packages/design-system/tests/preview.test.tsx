// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { PreviewAnchor, PreviewGroup } from "../index";

let host: HTMLDivElement;
let root: Root;
const tooltip = () => document.querySelector('[role="tooltip"]');
const tick = (ms: number) => act(() => vi.advanceTimersByTime(ms));
const pointer = (element: Element, type: string, pointerType = "mouse") =>
  act(() => {
    const event = new MouseEvent(type, { bubbles: true });
    Object.defineProperty(event, "pointerType", { value: pointerType });
    element.dispatchEvent(event);
  });
const mount = () =>
  act(() =>
    root.render(
      <PreviewGroup>
        <PreviewAnchor content={<p>Programme details</p>}>
          <button type="button">Programme</button>
        </PreviewAnchor>
        <PreviewAnchor content={<p>Clip identity</p>}>
          <button type="button">Break</button>
        </PreviewAnchor>
      </PreviewGroup>,
    ),
  );
const buttons = () => host.querySelectorAll("button");

beforeEach(() => {
  vi.useFakeTimers();
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  vi.stubGlobal(
    "ResizeObserver",
    class {
      observe() {}
      disconnect() {}
    },
  );
  vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockReturnValue(new DOMRect(900, 650, 100, 40));
  host = document.createElement("div");
  document.body.append(host);
  root = createRoot(host);
  mount();
});
afterEach(() => {
  act(() => root.unmount());
  host.remove();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

describe("contextual previews", () => {
  it("waits for intent and cancels a passing pointer", () => {
    pointer(buttons()[0]!, "pointerover");
    tick(200);
    expect(tooltip()).toBeNull();
    pointer(buttons()[0]!, "pointerout");
    tick(400);
    expect(tooltip()).toBeNull();
  });
  it("allows transfer to the preview, then dismisses after departure", () => {
    pointer(buttons()[0]!, "pointerover");
    tick(250);
    const panel = tooltip()!;
    expect(panel.textContent).toBe("Programme details");
    pointer(buttons()[0]!, "pointerout");
    pointer(panel, "pointerover");
    tick(300);
    expect(tooltip()).toBe(panel);
    pointer(panel, "pointerout");
    tick(200);
    expect(tooltip()).toBeNull();
  });
  it("describes actual focus, leaves focus on the block, and stays dismissed after Escape", () => {
    act(() => buttons()[0]!.focus());
    tick(250);
    expect(document.activeElement).toBe(buttons()[0]);
    expect(buttons()[0]!.getAttribute("aria-describedby")).toBe(tooltip()!.id);
    act(() => document.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true })));
    tick(1000);
    expect(tooltip()).toBeNull();
    expect(document.activeElement).toBe(buttons()[0]);
    expect(buttons()[0]!.hasAttribute("aria-describedby")).toBe(false);
  });
  it("keeps only one preview when focus moves to another block", () => {
    act(() => buttons()[0]!.focus());
    tick(250);
    act(() => buttons()[1]!.focus());
    tick(250);
    expect(document.querySelectorAll('[role="tooltip"]')).toHaveLength(1);
    expect(tooltip()!.textContent).toBe("Clip identity");
  });
  it("flips above a bottom-edge anchor, clamps the right edge, and closes on grid scroll", () => {
    act(() => buttons()[0]!.focus());
    tick(250);
    const panel = tooltip() as HTMLElement;
    vi.spyOn(panel, "getBoundingClientRect").mockReturnValue(new DOMRect(0, 0, 360, 300));
    act(() => window.dispatchEvent(new Event("resize")));
    expect(Number.parseFloat(panel.style.left)).toBe(window.innerWidth - 368);
    expect(Number.parseFloat(panel.style.top)).toBe(350);
    act(() => window.dispatchEvent(new Event("scroll")));
    expect(tooltip()).toBeNull();
  });
  it("does not impose hover previews on touch input", () => {
    pointer(buttons()[0]!, "pointerover", "touch");
    pointer(buttons()[0]!, "pointerdown", "touch");
    act(() => buttons()[0]!.focus());
    tick(1000);
    expect(tooltip()).toBeNull();
  });
});
