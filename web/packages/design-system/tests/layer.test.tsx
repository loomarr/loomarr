// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { renderToStaticMarkup } from "react-dom/server";
import { Animated } from "react-native";
import { afterEach, describe, expect, it, vi } from "vitest";

import { Layer, type LayerProps } from "../index";

afterEach(cleanup);

const drawn = (props: LayerProps) => {
  cleanup();
  render(<Layer testID="layer" {...props} />);
  return screen.getByTestId("layer");
};
const style = (props: LayerProps) => getComputedStyle(drawn(props));

describe("Layer", () => {
  it("is placed by its insets, in points or shares", () => {
    const placed = style({ bottom: 1, left: "25%", right: 3, top: 4 });
    expect(placed.position).toBe("absolute");
    expect([placed.top, placed.right, placed.bottom, placed.left]).toEqual(["4px", "3px", "1px", "25%"]);
  });

  it("covers its parent when it fills", () => {
    const filled = style({ fill: true });
    expect([filled.top, filled.right, filled.bottom, filled.left]).toEqual(["0px", "0px", "0px", "0px"]);
  });

  it("stays in the flow as the parent of other layers", () => {
    expect(style({ inFlow: true }).position).toBe("relative");
  });

  it("takes its size, offset and stacking", () => {
    const sized = style({ height: 3, marginLeft: -1, minWidth: 44, width: "100%", zIndex: 2 });
    expect([sized.height, sized.width, sized.minWidth]).toEqual(["3px", "100%", "44px"]);
    expect([sized.marginLeft, sized.zIndex]).toEqual(["-1px", "2"]);
  });

  it("centres itself across its parent", () => {
    const centred = style({ centred: true });
    expect(centred.left).toBe("50%");
    expect(centred.transform).toBe("translateX(-50%)");
  });

  it("lays children along a row and shares the room", () => {
    const row = style({ direction: "row", flex: 1 });
    expect(row.flexDirection).toBe("row");
    expect(row.flexGrow).toBe("1");
  });

  it("paints a named colour", () => {
    expect(style({ background: "chroma0" }).backgroundColor).toBe("rgb(255, 176, 32)");
    expect(style({ background: "playerSurface" }).backgroundColor).toBe("rgb(11, 12, 14)");
  });

  it("draws a still or an animated opacity", () => {
    expect(style({ opacity: 0.5 }).opacity).toBe("0.5");
    expect(drawn({ opacity: new Animated.Value(0.4) }).style.opacity).toBe("0.4");
  });

  it("lets the pointer through when asked", () => {
    expect(style({ pointerEvents: "none" }).pointerEvents).toBe("none");
  });

  it("carries its accessibility", () => {
    expect(drawn({ "aria-hidden": true }).getAttribute("aria-hidden")).toBe("true");
    const named = drawn({ "aria-label": "Live", role: "img" });
    expect(named.getAttribute("aria-label")).toBe("Live");
    expect(named.getAttribute("role")).toBe("img");
  });

  it("fades its trailing edge over the width given", () => {
    // jsdom drops a mask from an element's style, so the markup is what shows it.
    expect(renderToStaticMarkup(<Layer trailingFade={90} />)).toContain("calc(100% - 90px)");
  });

  it("fades its leading edge over the width given, alone or with its trailing edge", () => {
    // jsdom drops a mask from an element's style, so the markup is what shows it.
    const leading = renderToStaticMarkup(<Layer leadingFade={40} />);
    expect(leading).toContain("transparent, black 40px, black)");
    expect(leading).not.toContain("calc(");
    const both = renderToStaticMarkup(<Layer leadingFade={40} trailingFade={90} />);
    expect(both).toContain("transparent, black 40px, black calc(100% - 90px), transparent)");
  });

  it("hands on key events and its measured layout", () => {
    const onKeyDown = vi.fn();
    const onLayout = vi.fn();
    const host = drawn({ onKeyDown, onLayout }) as HTMLElement & {
      __reactLayoutHandler?: (event: unknown) => void;
    };
    fireEvent.keyDown(host, { key: "Home" });
    expect(onKeyDown).toHaveBeenCalledOnce();
    host.__reactLayoutHandler?.({ nativeEvent: { layout: { width: 200 } } });
    expect(onLayout).toHaveBeenCalledOnce();
  });
});
