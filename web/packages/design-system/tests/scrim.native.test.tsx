// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import { createElement, useEffect } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { Scrim } from "../index";

// The native adapter (scrim.tsx) draws once its box is measured; jsdom measures nothing, so the
// View reports a 400 by 100 box.
vi.mock("react-native", async (original) => {
  const native = await original<typeof import("react-native")>();
  const View = ({ onLayout, ...props }: React.ComponentProps<typeof native.View>) => {
    // Once, as a layout is reported once for a box that does not change.
    // biome-ignore lint/correctness/useExhaustiveDependencies: a new callback is not a new layout
    useEffect(() => {
      onLayout?.({ nativeEvent: { layout: { height: 100, width: 400, x: 0, y: 0 } } } as never);
    }, []);
    return createElement(native.View, props);
  };
  return { ...native, View };
});

afterEach(cleanup);

const stopsOf = (gradient: Element) =>
  [...gradient.querySelectorAll("stop")].map((stop) => [
    stop.getAttribute("offset"),
    stop.getAttribute("stop-color"),
    stop.getAttribute("stop-opacity"),
  ]);

const gradients = () => [...screen.getByTestId("scrim").querySelectorAll<Element>("linearGradient")];

describe("Scrim (native)", () => {
  it("draws the ink as a gradient over the feather, not a solid box", () => {
    render(<Scrim feather={64} from="right" testID="scrim" />);
    const [ink] = gradients() as [Element];
    expect([ink.getAttribute("x1"), ink.getAttribute("x2")]).toEqual(["336", "400"]);
    expect(stopsOf(ink)).toEqual([
      ["0", "#0B0C0E", "0.95"],
      ["1", "#0B0C0E", "0"],
    ]);
  });

  it("fades toward the top over the feather", () => {
    render(<Scrim feather={64} from="top" testID="scrim" />);
    const [ink] = gradients() as [Element];
    expect([ink.getAttribute("y1"), ink.getAttribute("y2")]).toEqual(["64", "0"]);
  });

  it("masks its top and bottom where it softens", () => {
    render(<Scrim feather={64} from="right" soften={25} testID="scrim" />);
    const fade = gradients()[1] as Element;
    expect(stopsOf(fade).map(([offset, , opacity]) => [offset, opacity])).toEqual([
      ["0", "0"],
      ["0.25", "1"],
      ["0.75", "1"],
      ["1", "0"],
    ]);
    expect(screen.getByTestId("scrim").querySelector("rect[mask]")).not.toBeNull();
  });
});
