// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { LoomarrProvider, Text } from "../index";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const itemOf = (id: string) => {
  const style = getComputedStyle(screen.getByTestId(id));
  return { basis: style.flexBasis, grow: style.flexGrow, minWidth: style.minWidth, shrink: style.flexShrink };
};

// `flex-1`, `shrink-0` with `min-w-0`, and a `flex-1` that must not give way.
const cases = [{ flex: 1 }, { flexShrink: 0, minWidth: 0 }, { flex: 2, flexShrink: 0 }] as const;

describe("Text as a flex item", () => {
  it.each(cases)("sizes a Tamagui text and an `as` host the same way: %o", (item) => {
    const error = vi.spyOn(console, "error").mockImplementation(() => undefined);
    render(
      <LoomarrProvider theme="dark">
        <Text data-testid="tamagui" textRole="body" {...item}>
          A title
        </Text>
        <Text as="span" data-testid="host" textRole="body" {...item}>
          A title
        </Text>
      </LoomarrProvider>,
    );

    const host = screen.getByTestId("host");
    expect(host.tagName).toBe("SPAN");
    expect(itemOf("host")).toEqual(itemOf("tamagui"));
    // The props become style on the host, not attributes the browser ignores.
    for (const attribute of ["flex", "flexshrink", "minwidth"])
      expect(host.hasAttribute(attribute)).toBe(false);
    expect(error).not.toHaveBeenCalled();
  });

  it("grows into a row's free space and shrinks below its longest word", () => {
    render(
      <LoomarrProvider theme="dark">
        <Text data-testid="tamagui" flex={1} minWidth={0} textRole="body">
          A title
        </Text>
      </LoomarrProvider>,
    );

    expect(itemOf("tamagui")).toEqual({ basis: "0px", grow: "1", minWidth: "0px", shrink: "1" });
  });

  it("sizes a haloed text's wrapper, the element its row lays out, and not the copies inside it", () => {
    render(
      <LoomarrProvider theme="dark">
        <div data-testid="row" style={{ display: "flex" }}>
          <Text density="tv" flex={1} halo minWidth={0} textRole="metadata">
            TUNING IN
          </Text>
        </div>
      </LoomarrProvider>,
    );

    const wrapper = screen.getByTestId("row").firstElementChild as HTMLElement;
    const wrapperStyle = getComputedStyle(wrapper);
    expect([wrapperStyle.flexGrow, wrapperStyle.minWidth]).toEqual(["1", "0px"]);
    const copies = [...wrapper.children].map((copy) => getComputedStyle(copy).flexGrow);
    expect(copies).toHaveLength(2);
    expect(copies.every((grow) => grow !== "1")).toBe(true);
  });

  it("leaves a text with none of the props at its own defaults", () => {
    render(
      <LoomarrProvider theme="dark">
        <Text as="span" data-testid="host" textRole="body">
          plain
        </Text>
      </LoomarrProvider>,
    );

    const { style } = screen.getByTestId("host");
    expect([style.flexGrow, style.flexShrink, style.flexBasis, style.minWidth]).toEqual(["", "", "", ""]);
  });
});
