// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Text } from "react-native";
import { afterEach, describe, expect, it, vi } from "vitest";

import { declarationsOfElement, declarationsOfTree } from "../atomic-css";
import { LoomarrProvider, RingedPressable, type RingedPressableProps } from "../index";

const nativeMatches = Element.prototype.matches;
afterEach(() => {
  cleanup();
  Element.prototype.matches = nativeMatches;
});

const draw = (props: Partial<RingedPressableProps> = {}) =>
  render(
    <LoomarrProvider theme="dark">
      <RingedPressable accessibilityLabel="Pause" size={44} {...props}>
        <Text>II</Text>
      </RingedPressable>
    </LoomarrProvider>,
  );

const pause = () => screen.getByRole("button", { name: "Pause" });

describe("RingedPressable", () => {
  it("reports presses and says whether what it opens is open", async () => {
    const onPress = vi.fn();
    draw({ expanded: true, onPress });
    expect(pause().getAttribute("aria-expanded")).toBe("true");
    await userEvent.click(pause());
    expect(onPress).toHaveBeenCalledTimes(1);
  });

  it("turns the browser's outline off and draws the ring itself", async () => {
    Element.prototype.matches = new Proxy(nativeMatches, {
      apply: (original, element, args: [string]) =>
        args[0] === ":focus-visible" || Reflect.apply(original, element, args),
    });
    draw();
    await userEvent.tab();
    expect(getComputedStyle(pause()).outlineStyle).toBe("none");
    expect(screen.getByTestId("ring-actionFocus")).toBeTruthy();
  });

  it("is a button unless named an option or a tab, which say whether they are selected", () => {
    draw();
    expect(pause().getAttribute("aria-selected")).toBeNull();
    cleanup();
    draw({ role: "option", selected: true });
    expect(screen.getByRole("option", { name: "Pause", selected: true })).toBeTruthy();
    cleanup();
    draw({ role: "tab" });
    expect(screen.getByRole("tab", { name: "Pause" }).getAttribute("aria-selected")).toBe("false");
  });

  it("reports keyboard focus alone, and lets it go on a pointer press and on leaving", async () => {
    let keyboard = false;
    Element.prototype.matches = new Proxy(nativeMatches, {
      apply: (original, element, args: [string]) =>
        args[0] === ":focus-visible" ? keyboard : Reflect.apply(original, element, args),
    });
    const onKeyboardFocus = vi.fn();
    const view = draw({ onKeyboardFocus });
    // Focus that followed a click is not the keyboard's.
    await userEvent.click(pause());
    expect(onKeyboardFocus).not.toHaveBeenCalled();
    // A key on the control it holds brings the keyboard's ring.
    keyboard = true;
    await userEvent.keyboard("{Shift}");
    expect(onKeyboardFocus.mock.calls).toEqual([[true]]);
    fireEvent.pointerDown(pause());
    expect(onKeyboardFocus.mock.calls).toEqual([[true], [false]]);
    await userEvent.keyboard("{Shift}");
    expect(onKeyboardFocus.mock.calls).toEqual([[true], [false], [true]]);
    view.unmount();
    expect(onKeyboardFocus.mock.calls).toEqual([[true], [false], [true], [false]]);
  });

  it("is named by its words when no label is given", () => {
    render(
      <LoomarrProvider theme="dark">
        <RingedPressable role="option" size={44}>
          <Text>Coast 90s</Text>
        </RingedPressable>
      </LoomarrProvider>,
    );
    expect(screen.getByRole("option", { name: "Coast 90s" })).toBeTruthy();
  });

  it("calls a caller's hover handlers beside its own hover drawing", () => {
    const onHoverIn = vi.fn();
    const onHoverOut = vi.fn();
    draw({ onHoverIn, onHoverOut });
    fireEvent.pointerEnter(pause(), { pointerType: "mouse" });
    expect(onHoverIn).toHaveBeenCalledTimes(1);
    expect(screen.getByTestId("ring-contentPrimary")).toBeTruthy();
    fireEvent.pointerLeave(pause(), { pointerType: "mouse" });
    expect(onHoverOut).toHaveBeenCalledTimes(1);
    expect(screen.getByTestId("ring-transparent")).toBeTruthy();
  });

  it("draws the hover look by the caller's word when told whether it is highlighted", () => {
    const view = draw({ highlighted: false });
    fireEvent.pointerEnter(pause(), { pointerType: "mouse" });
    expect(screen.getByTestId("ring-transparent")).toBeTruthy();
    expect(declarationsOfTree(pause())).not.toContain("var(--surfaceElevated)");
    view.rerender(
      <LoomarrProvider theme="dark">
        <RingedPressable accessibilityLabel="Pause" highlighted size={44}>
          <Text>II</Text>
        </RingedPressable>
      </LoomarrProvider>,
    );
    expect(declarationsOfTree(pause())).toContain("var(--surfaceElevated)");
  });

  it("hands the mousedown to the caller, which can keep focus where it is", async () => {
    draw({ onMouseDown: (event) => event.preventDefault() });
    await userEvent.click(pause());
    expect(document.activeElement).toBe(document.body);
  });

  it("lays its face from the leading edge across the whole width when asked", () => {
    draw({ align: "start", fill: true });
    expect(getComputedStyle(pause()).width).toBe("100%");
    expect(declarationsOfElement(screen.getByTestId("ring-transparent"))).toContain("flex-start");
    cleanup();
    draw();
    expect(getComputedStyle(pause()).width).not.toBe("100%");
    expect(declarationsOfElement(screen.getByTestId("ring-transparent"))).not.toContain("flex-start");
  });

  it("names itself in a tooltip on hover", async () => {
    draw({ tip: "Pause (Space)" });
    await userEvent.hover(pause());
    expect((await screen.findByRole("tooltip")).textContent).toBe("Pause (Space)");
  });
});
