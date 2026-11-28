// @vitest-environment jsdom

import { cleanup, renderUi } from "@loomarr/test-support";
import { act } from "react";
import { AccessibilityInfo, Animated } from "react-native";
import { afterEach, describe, expect, it, vi } from "vitest";

import { TuneBars, TuneStatic } from "../index";

// Each step also lets the platform's reduce-motion answer arrive before the test reads the screen.
const mount = async (node: React.ReactNode, theme: "dark" | "light" = "dark") => {
  const { container, rerender } = renderUi(node, { theme });
  const render = (next: React.ReactNode) => act(async () => rerender(next));
  await act(async () => {});
  return { host: container, render };
};

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const part = (host: HTMLElement) => host.querySelector<HTMLElement>('[data-testid^="tune-static-"]');
// The snow is the only thing that draws an SVG.
const snow = (host: HTMLElement) => host.querySelector("svg");

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

  it("flickers only while covering, starting with the cover and without asking the platform", async () => {
    const loops: { start: ReturnType<typeof vi.fn>; stop: ReturnType<typeof vi.fn> }[] = [];
    vi.spyOn(Animated, "loop").mockImplementation(() => {
      const loop = { reset: vi.fn(), start: vi.fn(), stop: vi.fn() };
      loops.push(loop);
      return loop;
    });
    const setting = vi.spyOn(AccessibilityInfo, "isReduceMotionEnabled");
    const { render } = await mount(<TuneStatic cover="none" reducedMotion={false} />);
    expect(loops).toHaveLength(0);
    await render(<TuneStatic cover="burst" reducedMotion={false} />);
    expect(loops).toHaveLength(1);
    expect(loops[0]?.start).toHaveBeenCalledOnce();
    await render(<TuneStatic cover="none" reducedMotion={false} />);
    expect(loops[0]?.stop).toHaveBeenCalled();
    expect(loops).toHaveLength(1);
    await render(<TuneStatic cover="wash" holdSnow reducedMotion={false} />);
    expect(loops).toHaveLength(1);
    expect(setting).not.toHaveBeenCalled();
  });

  it("crossfades the darkened picture, with no static, under reduced motion", async () => {
    // The test platform finishes an animation at once, so the fade shows as the timing it asks for.
    const timing = vi.spyOn(Animated, "timing");
    const fades = () => timing.mock.calls.map(([, { duration, toValue }]) => ({ duration, toValue }));
    const { host, render } = await mount(<TuneStatic cover="burst" reducedMotion />);
    const opacity = () => Number(part(host)?.style.opacity);
    expect(snow(host)).toBeNull();
    const veil = part(host)?.firstElementChild as HTMLElement | undefined;
    expect(veil?.style.backgroundColor).toBe("rgb(11, 12, 14)");
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
  const bars = (host: HTMLElement) => host.querySelector('[data-testid^="tune-bars-"]');
  const colours = (host: HTMLElement) =>
    Array.from(bars(host)?.children ?? [], (bar) =>
      (bar as HTMLElement).style.backgroundColor.replace(/rgba?\((\d+), (\d+), (\d+).*/, "$1,$2,$3"),
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
    const { host, render } = await mount(<TuneBars bars="waiting" run={1} reducedMotion={false} />);
    expect(colours(host)).toHaveLength(7);
    expect(new Set(colours(host))).toEqual(new Set(["139,147,163", "90,97,112"]));
    await render(<TuneBars bars="locked" run={1} reducedMotion={false} />);
    expect(colours(host)).toEqual(chroma);
  });

  it("runs the seven colours on a flip, and does not run them under reduced motion", async () => {
    const { host, render } = await mount(<TuneBars bars="run" run={1} reducedMotion={false} />);
    expect(colours(host)).toEqual(chroma);
    await render(<TuneBars bars="run" run={1} reducedMotion />);
    expect(bars(host)).toBeNull();
    await render(<TuneBars bars="waiting" run={1} reducedMotion />);
    expect(bars(host)).not.toBeNull();
    await render(<TuneBars bars="none" run={1} reducedMotion={false} />);
    expect(bars(host)).toBeNull();
  });
});
