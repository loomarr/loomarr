// @vitest-environment jsdom

import { LoomarrProvider } from "@loomarr/design-system";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { AccessibilityInfo, Animated } from "react-native";
import { afterEach, describe, expect, it, vi } from "vitest";

import { TuneBars, TuneStatic } from "../index";

(
  globalThis as typeof globalThis & {
    IS_REACT_ACT_ENVIRONMENT: boolean;
  }
).IS_REACT_ACT_ENVIRONMENT = true;

// The package compiles without the DOM library, so the tests name what they read.
type DomElement = {
  children: Iterable<DomElement>;
  firstElementChild: DomElement;
  getAttribute: (name: string) => string | null;
  innerHTML: string;
  parentElement: DomElement;
  querySelector: (selector: string) => DomElement | null;
  style: { backgroundColor: string; opacity: string };
};
const { document } = globalThis as unknown as {
  document: {
    body: { appendChild: (node: unknown) => void; replaceChildren: () => void };
    createElement: (tagName: string) => Parameters<typeof createRoot>[0] & DomElement;
  };
};

const roots: Root[] = [];

const mount = async (node: React.ReactNode, theme: "dark" | "light" = "dark") => {
  const host = document.createElement("div");
  document.body.appendChild(host);
  const root = createRoot(host);
  roots.push(root);
  const render = (next: React.ReactNode) =>
    act(async () => root.render(<LoomarrProvider theme={theme}>{next}</LoomarrProvider>));
  await render(node);
  return { host, render };
};

afterEach(() => {
  for (const root of roots.splice(0)) act(() => root.unmount());
  document.body.replaceChildren();
  vi.restoreAllMocks();
});

const part = (host: DomElement) => host.querySelector('[data-testid^="tune-static-"]');
// The snow is the only thing that draws an SVG.
const snow = (host: DomElement) => host.querySelector("svg");

describe("TuneStatic", () => {
  it("hides its own wrapper from assistive technology", async () => {
    const { host } = await mount(<TuneStatic cover="burst" reducedMotion={false} />);
    expect(part(host)?.getAttribute("aria-hidden")).toBe("true");
  });

  it("keeps one snow mounted, hidden while clear, across covers", async () => {
    const { host, render } = await mount(<TuneStatic cover="none" reducedMotion={false} />);
    const first = snow(host);
    expect(first).not.toBeNull();
    expect(part(host)?.style.opacity).toBe("0");
    for (const cover of ["burst", "none", "wash", "none", "cold"] as const) {
      await render(<TuneStatic cover={cover} reducedMotion={false} />);
      expect(snow(host), cover).toBe(first);
    }
    expect(part(host)?.style.opacity).toBe("0.8");
  });

  it("crossfades the darkened picture, with no static, under reduced motion", async () => {
    // The test platform finishes an animation at once, so the fade shows as the timing it asks for.
    const timing = vi.spyOn(Animated, "timing");
    const fades = () => timing.mock.calls.map(([, { duration, toValue }]) => ({ duration, toValue }));
    const { host, render } = await mount(<TuneStatic cover="burst" reducedMotion />);
    const opacity = () => Number(part(host)?.style.opacity);
    expect(snow(host)).toBeNull();
    expect(part(host)?.firstElementChild.style.backgroundColor).toBe("rgb(11, 12, 14)");
    expect(fades()).toEqual([{ duration: 120, toValue: 0.88 }]);
    await vi.waitFor(() => expect(opacity()).toBe(0.88));
    await render(<TuneStatic cover="none" reducedMotion />);
    expect(fades().at(-1)).toEqual({ duration: 120, toValue: 0 });
    await vi.waitFor(() => expect(opacity()).toBe(0));
  });

  it("follows the platform's reduce-motion setting", async () => {
    const setting = vi.spyOn(AccessibilityInfo, "isReduceMotionEnabled");
    setting.mockResolvedValue(true);
    const reduced = await mount(<TuneStatic cover="wash" />);
    expect(snow(reduced.host)).toBeNull();
    setting.mockResolvedValue(false);
    const moving = await mount(<TuneStatic cover="wash" />);
    expect(snow(moving.host)).not.toBeNull();
  });

  it("draws no static while the setting is still unknown", async () => {
    vi.spyOn(AccessibilityInfo, "isReduceMotionEnabled").mockReturnValue(new Promise(() => {}));
    const { host } = await mount(<TuneStatic cover="wash" />);
    expect(part(host)).not.toBeNull();
    expect(snow(host)).toBeNull();
  });

  it("keeps the same dim snow in the light theme", async () => {
    const wash = <TuneStatic cover="wash" reducedMotion={false} />;
    const dark = await mount(wash);
    const light = await mount(wash, "light");
    expect(snow(light.host)?.parentElement?.parentElement?.innerHTML).toBe(
      snow(dark.host)?.parentElement?.parentElement?.innerHTML,
    );
  });
});

describe("TuneBars", () => {
  const bars = (host: DomElement) => host.querySelector('[data-testid^="tune-bars-"]');
  const colours = (host: DomElement) =>
    Array.from(bars(host)?.children ?? [], (bar) =>
      bar.style.backgroundColor.replace(/rgba?\((\d+), (\d+), (\d+).*/, "$1,$2,$3"),
    );
  const chroma = [
    "255,176,32",
    "245,217,10",
    "61,214,140",
    "76,201,232",
    "214,64,159",
    "229,72,77",
    "139,147,163",
  ];

  it("waits in two greys and locks to the seven colours", async () => {
    const { host, render } = await mount(<TuneBars bars="waiting" flip={1} reducedMotion={false} />);
    expect(colours(host)).toHaveLength(7);
    expect(new Set(colours(host))).toEqual(new Set(["139,147,163", "90,97,112"]));
    await render(<TuneBars bars="locked" flip={1} reducedMotion={false} />);
    expect(colours(host)).toEqual(chroma);
  });

  it("runs the seven colours on a flip, and does not run them under reduced motion", async () => {
    const { host, render } = await mount(<TuneBars bars="run" flip={1} reducedMotion={false} />);
    expect(colours(host)).toEqual(chroma);
    await render(<TuneBars bars="run" flip={1} reducedMotion />);
    expect(bars(host)).toBeNull();
    await render(<TuneBars bars="waiting" flip={1} reducedMotion />);
    expect(bars(host)).not.toBeNull();
    await render(<TuneBars bars="none" flip={1} reducedMotion={false} />);
    expect(bars(host)).toBeNull();
  });
});
