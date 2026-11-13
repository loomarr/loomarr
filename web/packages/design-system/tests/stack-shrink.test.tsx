// @vitest-environment jsdom
import { act, cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { breakpoints, LoomarrProvider, Stack } from "../index";

// jsdom has no media queries. This stands in for a browser's: a `(min-width: Npx)` query matches
// while the window is at least N wide, and its listeners hear it when a resize flips the answer.
let windowWidth = 1280;
const listeners = new Map<MediaQueryList, Set<() => void>>();
const matchMedia = (query: string) => {
  const minimum = Number(query.match(/min-width:\s*(\d+)px/)?.[1]);
  const list = {
    addEventListener: (_: string, listener: () => void) => listeners.get(list)?.add(listener),
    get matches() {
      return windowWidth >= minimum;
    },
    media: query,
    removeEventListener: (_: string, listener: () => void) => listeners.get(list)?.delete(listener),
  } as unknown as MediaQueryList;
  listeners.set(list, new Set());
  return list;
};

beforeEach(() => {
  vi.stubGlobal("matchMedia", matchMedia);
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  listeners.clear();
});

const resizeTo = (width: number) =>
  act(() => {
    const before = new Map([...listeners.keys()].map((list) => [list, list.matches]));
    windowWidth = width;
    for (const [list, wasMatching] of before) {
      if (list.matches !== wasMatching) for (const listener of listeners.get(list) ?? []) listener();
    }
  });

const shrinkOf = (name: string) => getComputedStyle(screen.getByRole("group", { name })).flexShrink;

const inProvider = (node: React.ReactNode) => render(<LoomarrProvider theme="dark">{node}</LoomarrProvider>);

describe("Stack's default flexShrink follows the Stack around it", () => {
  it("is 0 in a column", () => {
    inProvider(
      <Stack>
        <Stack aria-label="child" role="group" />
      </Stack>,
    );
    expect(shrinkOf("child")).toBe("0");
  });

  it("is 1 in a row", () => {
    inProvider(
      <Stack direction="row">
        <Stack aria-label="child" role="group" />
      </Stack>,
    );
    expect(shrinkOf("child")).toBe("1");
  });

  it("is 1 with no Stack above it", () => {
    inProvider(<Stack aria-label="alone" role="group" />);
    expect(shrinkOf("alone")).toBe("1");
  });

  it("is the nearest Stack's, through an element that is not a Stack", () => {
    inProvider(
      <Stack direction="row">
        <Stack>
          <div>
            <Stack aria-label="child" role="group" />
          </div>
        </Stack>
      </Stack>,
    );
    expect(shrinkOf("child")).toBe("0");
  });

  it("lets an explicit flexShrink win in either direction", () => {
    inProvider(
      <Stack>
        <Stack aria-label="in a column" flexShrink={1} role="group" />
        <Stack direction="row">
          <Stack aria-label="in a row" flexShrink={0} role="group" />
        </Stack>
      </Stack>,
    );
    expect(shrinkOf("in a column")).toBe("1");
    expect(shrinkOf("in a row")).toBe("0");
  });

  it("follows a responsive direction as the window crosses its breakpoint", () => {
    const tree = (
      <Stack direction={{ base: "column", sm: "row" }}>
        <Stack aria-label="child" role="group" />
      </Stack>
    );
    resizeTo(breakpoints.sm - 1);
    inProvider(tree);
    expect(shrinkOf("child")).toBe("0");

    resizeTo(breakpoints.sm);
    expect(shrinkOf("child")).toBe("1");

    resizeTo(breakpoints.sm - 1);
    expect(shrinkOf("child")).toBe("0");
  });
});
