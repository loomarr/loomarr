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

const flexOf = (name: string) => {
  const element = screen.getByRole("group", { name });
  const style = getComputedStyle(element);
  return { align: style.alignItems, direction: style.flexDirection, justify: style.justifyContent };
};

// `flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between` as one Stack.
const Row = () => (
  <LoomarrProvider theme="dark">
    <Stack
      align={{ base: "stretch", sm: "start" }}
      aria-label="header"
      direction={{ base: "column", sm: "row" }}
      gap={{ base: "3", md: "4" }}
      justify={{ base: "start", sm: "between" }}
      role="group"
    >
      <span>title</span>
      <span>actions</span>
    </Stack>
  </LoomarrProvider>
);

describe("Stack with responsive values", () => {
  it("names the breakpoints as the app's own Tailwind widths", () => {
    expect(breakpoints).toEqual({ sm: 640, md: 768, lg: 1024 });
  });

  it("is the narrow layout below the first breakpoint and takes each rule over as the window reaches it", () => {
    resizeTo(breakpoints.sm - 1);
    render(<Row />);
    expect(flexOf("header")).toEqual({ align: "stretch", direction: "column", justify: "flex-start" });

    resizeTo(breakpoints.sm);
    expect(flexOf("header")).toEqual({ align: "flex-start", direction: "row", justify: "space-between" });

    // `md` only changes the gap; the `sm` layout stays.
    resizeTo(breakpoints.md);
    expect(flexOf("header")).toEqual({ align: "flex-start", direction: "row", justify: "space-between" });

    resizeTo(breakpoints.sm - 1);
    expect(flexOf("header").direction).toBe("column");
  });

  it("paints the right layout on the first render, not after a resize", () => {
    resizeTo(breakpoints.lg);
    render(<Row />);
    expect(flexOf("header").direction).toBe("row");
  });

  it("changes a wrapped row's per-axis gaps with the window too", () => {
    resizeTo(breakpoints.sm - 1);
    render(
      <LoomarrProvider theme="dark">
        <Stack
          aria-label="chips"
          columnGap={{ base: "2", sm: "6" }}
          direction="row"
          gap="1"
          role="group"
          rowGap={{ base: "none", md: "3" }}
          wrap
        />
      </LoomarrProvider>,
    );
    const gaps = () => {
      const style = getComputedStyle(screen.getByRole("group", { name: "chips" }));
      return [style.columnGap, style.rowGap];
    };
    expect(gaps()).toEqual(["var(--t-space-2)", "0px"]);

    resizeTo(breakpoints.sm);
    expect(gaps()).toEqual(["var(--t-space-6)", "0px"]);

    resizeTo(breakpoints.md);
    expect(gaps()).toEqual(["var(--t-space-6)", "var(--t-space-3)"]);
  });

  it("keeps a plain value the same at every width", () => {
    resizeTo(breakpoints.sm - 1);
    render(
      <LoomarrProvider theme="dark">
        <Stack aria-label="plain" direction="row" role="group" />
      </LoomarrProvider>,
    );
    expect(flexOf("plain").direction).toBe("row");
    resizeTo(breakpoints.lg);
    expect(flexOf("plain").direction).toBe("row");
  });
});
