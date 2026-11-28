// @vitest-environment jsdom
import { act, cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Pressable, Text } from "react-native";
import { afterEach, describe, expect, it } from "vitest";

import { useRingFocus } from "../index";

// jsdom answers `:focus-visible` with false whatever gave the focus. This stand-in follows the last
// input, as a browser does: a key makes focus visible and a pointer press does not.
const nativeMatches = Element.prototype.matches;
let keyed = true;
Element.prototype.matches = new Proxy(nativeMatches, {
  apply: (original, element, args: [string]) =>
    args[0] === ":focus-visible" ? keyed : Reflect.apply(original, element, args),
});
document.addEventListener("keydown", () => (keyed = true), true);
document.addEventListener("pointerdown", () => (keyed = false), true);

afterEach(() => {
  cleanup();
  keyed = true;
});

const Control = ({ name }: { name: string }) => {
  const { handlers, ring } = useRingFocus();
  return (
    <Pressable accessibilityRole="button" aria-label={name} {...handlers} testID={`${name}-${ring}`}>
      <Text>{name}</Text>
    </Pressable>
  );
};

const both = () =>
  render(
    <>
      <Control name="one" />
      <Control name="two" />
    </>,
  );

const button = (name: string) => screen.getByRole("button", { name });
const ringOf = (name: string) => button(name).getAttribute("data-testid")?.split("-")[1];

describe("useRingFocus", () => {
  it("is amber for keyboard focus and gone when focus leaves", async () => {
    both();
    await userEvent.tab();
    expect(ringOf("one")).toBe("actionFocus");
    await userEvent.tab();
    expect([ringOf("one"), ringOf("two")]).toEqual(["transparent", "actionFocus"]);
  });

  it("is plain under a pointer and leaves nothing after a click", async () => {
    both();
    await userEvent.click(button("one"));
    expect(ringOf("one")).toBe("contentPrimary");
    await userEvent.unhover(button("one"));
    expect(button("one")).toBe(document.activeElement);
    expect(ringOf("one")).toBe("transparent");
  });

  it("draws one ring: a pointer over another control draws none while the keyboard holds one", async () => {
    both();
    await userEvent.tab();
    await userEvent.hover(button("two"));
    expect([ringOf("one"), ringOf("two")]).toEqual(["actionFocus", "transparent"]);
    act(() => button("one").blur());
    expect(ringOf("two")).toBe("contentPrimary");
  });

  it("drops the amber ring on a pointer press of the focused control, until the next key", async () => {
    both();
    await userEvent.tab();
    await userEvent.click(button("one"));
    expect(ringOf("one")).toBe("contentPrimary");
    await userEvent.keyboard("{Shift}");
    expect(ringOf("one")).toBe("actionFocus");
  });

  it("releases its ring when it is removed while focused", async () => {
    const view = both();
    await userEvent.tab();
    view.rerender(
      <>
        <Control key="two" name="two" />
      </>,
    );
    await userEvent.hover(button("two"));
    expect(ringOf("two")).toBe("contentPrimary");
  });
});
