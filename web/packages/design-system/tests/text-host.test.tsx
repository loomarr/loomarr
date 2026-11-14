// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { LoomarrProvider, Text } from "../index";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const hostOf = (ui: React.ReactNode) => {
  render(<LoomarrProvider theme="dark">{ui}</LoomarrProvider>);
  return screen.getByTestId("host");
};

describe("Text with an `as` host", () => {
  it("draws textAlign, textDecorationLine and fontVariant as inline style, not attributes", () => {
    const error = vi.spyOn(console, "error").mockImplementation(() => undefined);
    const host = hostOf(
      <Text
        as="p"
        data-testid="host"
        fontVariant={["tabular-nums"]}
        textAlign="center"
        textDecorationLine="underline"
        textRole="body"
      >
        0:42
      </Text>,
    );

    expect(host.tagName).toBe("P");
    expect(host.style.textAlign).toBe("center");
    expect(host.style.textDecorationLine).toBe("underline");
    expect(host.style.fontVariant).toBe("tabular-nums");
    for (const attribute of ["textalign", "textdecorationline", "font-variant", "fontvariant"]) {
      expect(host.hasAttribute(attribute)).toBe(false);
    }
    expect(error).not.toHaveBeenCalled();
  });

  it("truncates to one line with an ellipsis for numberOfLines 1", () => {
    const host = hostOf(
      <Text as="span" data-testid="host" numberOfLines={1} textRole="body">
        A very long title
      </Text>,
    );

    expect(host.style.overflow).toBe("hidden");
    expect(host.style.textOverflow).toBe("ellipsis");
    expect(host.style.whiteSpace).toBe("nowrap");
    expect(host.hasAttribute("numberoflines")).toBe(false);
  });

  it("clamps to N lines for numberOfLines above 1", () => {
    const host = hostOf(
      <Text as="p" data-testid="host" numberOfLines={3} textRole="body">
        A long description
      </Text>,
    );

    expect(host.style.display).toBe("-webkit-box");
    expect(host.style.overflow).toBe("hidden");
    expect(host.style.getPropertyValue("-webkit-line-clamp")).toBe("3");
    expect(host.style.getPropertyValue("-webkit-box-orient")).toBe("vertical");
    expect(host.style.whiteSpace).toBe("");
  });

  it("breaks a long unbroken word with wordBreak and overflowWrap", () => {
    const host = hostOf(
      <Text as="p" data-testid="host" overflowWrap="anywhere" textRole="body" wordBreak="break-word">
        a-very-long-unbroken-identifier
      </Text>,
    );

    expect(host.style.overflowWrap).toBe("anywhere");
    expect(host.style.wordBreak).toBe("break-word");
    expect(host.hasAttribute("overflowwrap")).toBe(false);
    expect(host.hasAttribute("wordbreak")).toBe(false);
  });

  it("adds nothing when none of the props is set, and lets the caller's style win", () => {
    const host = hostOf(
      <Text as="p" data-testid="host" style={{ textAlign: "right" }} textAlign="center" textRole="body">
        x
      </Text>,
    );

    expect(host.style.textAlign).toBe("right");
    expect(host.style.overflow).toBe("");
  });
});
