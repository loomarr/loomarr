// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Text } from "react-native";
import { afterEach, describe, expect, it, vi } from "vitest";

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

  it("names itself in a tooltip on hover", async () => {
    draw({ tip: "Pause (Space)" });
    await userEvent.hover(pause());
    expect((await screen.findByRole("tooltip")).textContent).toBe("Pause (Space)");
  });
});
