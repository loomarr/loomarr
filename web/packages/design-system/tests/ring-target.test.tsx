// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Text } from "react-native";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { LoomarrProvider, RingTarget, type RingTargetProps } from "../index";

const nativeMatches = Element.prototype.matches;
// The keyboard is the last thing used, so the browser's own test says focus is visible.
let keys = true;
beforeEach(() => {
  keys = true;
  Element.prototype.matches = new Proxy(nativeMatches, {
    apply: (original, element, args: [string]) =>
      args[0] === ":focus-visible" ? keys : Reflect.apply(original, element, args),
  });
});
afterEach(() => {
  cleanup();
  Element.prototype.matches = nativeMatches;
});

const draw = (props: Partial<RingTargetProps> = {}) =>
  render(
    <LoomarrProvider theme="dark">
      <button type="button">Elsewhere</button>
      <RingTarget accessibilityLabel="Block" {...props}>
        {({ keyboard, ring }) => <Text>{`${ring} ${keyboard}`}</Text>}
      </RingTarget>
    </LoomarrProvider>,
  );

const block = () => screen.getByRole("button", { name: "Block" });

describe("RingTarget", () => {
  it("draws amber for keyboard focus, with the browser's outline off", async () => {
    const onKeyboardFocus = vi.fn();
    const onFocus = vi.fn();
    draw({ onFocus, onKeyboardFocus });
    await userEvent.tab();
    await userEvent.tab();
    expect(block().textContent).toBe("actionFocus true");
    expect(getComputedStyle(block()).outlineStyle).toBe("none");
    expect(onFocus).toHaveBeenLastCalledWith(true);
    expect(onKeyboardFocus.mock.calls).toEqual([[true]]);
    await userEvent.tab({ shift: true });
    expect(onKeyboardFocus.mock.calls).toEqual([[true], [false]]);
  });

  it("draws the plain ring for a pointer and leaves none behind after a click", async () => {
    keys = false;
    const onPress = vi.fn();
    draw({ onPress });
    await userEvent.hover(block());
    expect(block().textContent).toBe("contentPrimary false");
    await userEvent.click(block());
    await userEvent.unhover(block());
    expect(onPress).toHaveBeenCalledTimes(1);
    expect(block().textContent).toBe("transparent false");
  });

  it("takes no focus from a pointer when asked", async () => {
    draw({ pointerTakesNoFocus: true });
    const elsewhere = screen.getByRole("button", { name: "Elsewhere" });
    await userEvent.click(elsewhere);
    await userEvent.click(block());
    expect(document.activeElement).toBe(elsewhere);
  });

  it("says whether what it opens is open, and takes the whole width when asked", () => {
    draw({ expanded: true, fill: true });
    expect(block().getAttribute("aria-expanded")).toBe("true");
    expect(getComputedStyle(block()).width).toBe("100%");
    expect(getComputedStyle(block()).outlineStyle).toBe("none");
  });

  it("hands the pressable itself to what is put around it", () => {
    draw({ wrap: (control) => <section aria-label="Around">{control}</section> });
    expect(screen.getByRole("region", { name: "Around" }).firstElementChild).toBe(block());
  });

  it("says what it is and whether it is selected", () => {
    draw({ role: "tab", selected: true, tabIndex: -1 });
    const tab = screen.getByRole("tab", { name: "Block" });
    expect([tab.getAttribute("aria-selected"), tab.getAttribute("tabindex")]).toEqual(["true", "-1"]);
  });
});
