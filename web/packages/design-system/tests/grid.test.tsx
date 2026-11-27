// @vitest-environment jsdom
import { act, cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { breakpoints, Grid, GridItem, LoomarrProvider } from "../index";

// jsdom has no media queries: this stands in for a browser's, as the Stack tests do.
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

const grid = () => screen.getByRole("group", { name: "grid" });

// `grid grid-cols-1 gap-x-4 gap-y-5 sm:grid-cols-2`, with one field that spans both columns.
const Fields = () => (
  <LoomarrProvider theme="dark">
    <Grid aria-label="grid" columnGap="4" columns={{ base: 1, sm: 2 }} role="group" rowGap="5">
      <div>one</div>
      <div>two</div>
      <GridItem data-testid="wide" span={{ base: 1, sm: "full" }}>
        wide
      </GridItem>
    </Grid>
  </LoomarrProvider>
);

describe("Grid on the web", () => {
  it("is a CSS grid of equal columns that follows the window", () => {
    resizeTo(breakpoints.sm - 1);
    render(<Fields />);
    expect(getComputedStyle(grid()).display).toBe("grid");
    expect(getComputedStyle(grid()).gridTemplateColumns).toBe("repeat(1, minmax(0, 1fr))");

    resizeTo(breakpoints.sm);
    expect(getComputedStyle(grid()).gridTemplateColumns).toBe("repeat(2, minmax(0, 1fr))");

    resizeTo(breakpoints.sm - 1);
    expect(getComputedStyle(grid()).gridTemplateColumns).toBe("repeat(1, minmax(0, 1fr))");
  });

  it("paints the wide layout on the first render", () => {
    resizeTo(breakpoints.lg);
    render(<Fields />);
    expect(getComputedStyle(grid()).gridTemplateColumns).toBe("repeat(2, minmax(0, 1fr))");
  });

  it("spaces rows and columns from the spacing tokens", () => {
    render(<Fields />);
    const style = getComputedStyle(grid());
    expect(style.columnGap).toBe("var(--t-space-4)");
    expect(style.rowGap).toBe("var(--t-space-5)");
  });

  it("uses one gap for both axes, and none when it is left out", () => {
    render(
      <LoomarrProvider theme="dark">
        <Grid aria-label="grid" columns={2} gap="3" role="group" />
      </LoomarrProvider>,
    );
    expect(getComputedStyle(grid()).columnGap).toBe("var(--t-space-3)");
    expect(getComputedStyle(grid()).rowGap).toBe("var(--t-space-3)");
    cleanup();
    render(
      <LoomarrProvider theme="dark">
        <Grid aria-label="grid" columns={2} role="group" />
      </LoomarrProvider>,
    );
    expect(getComputedStyle(grid()).rowGap).toBe("0px");
  });

  it("spans an item across columns, and `full` is every column in force", () => {
    resizeTo(breakpoints.sm - 1);
    render(<Fields />);
    expect(screen.getByTestId("wide").style.gridColumn).toBe("span 1 / span 1");

    resizeTo(breakpoints.sm);
    expect(screen.getByTestId("wide").style.gridColumn).toBe("span 2 / span 2");
  });

  it("draws a list of tracks as written, and changes it with the window", () => {
    resizeTo(breakpoints.md - 1);
    render(
      <LoomarrProvider theme="dark">
        <Grid
          aria-label="grid"
          columns={{ base: 1, md: ["17rem", "1fr"], lg: ["auto", "54px", "minmax(12rem,1fr)", "1.5fr"] }}
          role="group"
        />
      </LoomarrProvider>,
    );
    expect(getComputedStyle(grid()).gridTemplateColumns).toBe("repeat(1, minmax(0, 1fr))");

    resizeTo(breakpoints.md);
    expect(getComputedStyle(grid()).gridTemplateColumns).toBe("17rem 1fr");

    resizeTo(breakpoints.lg);
    expect(getComputedStyle(grid()).gridTemplateColumns).toBe("auto 54px minmax(12rem,1fr) 1.5fr");
  });

  it("clamps a span to the tracks in a list", () => {
    render(
      <LoomarrProvider theme="dark">
        <Grid columns={["auto", "1fr"]}>
          <GridItem data-testid="wide" span="full">
            x
          </GridItem>
        </Grid>
      </LoomarrProvider>,
    );
    expect(screen.getByTestId("wide").style.gridColumn).toBe("span 2 / span 2");
  });

  it("starts an item in a named column and runs it to the end with `full`", () => {
    render(
      <LoomarrProvider theme="dark">
        <Grid columns={["auto", "1fr", "auto"]}>
          <GridItem data-testid="rest" from={2} span="full">
            rest
          </GridItem>
          <GridItem data-testid="two" from={2} span={2}>
            two
          </GridItem>
        </Grid>
      </LoomarrProvider>,
    );
    expect(screen.getByTestId("rest").style.gridColumn).toBe("2 / -1");
    expect(screen.getByTestId("two").style.gridColumn).toBe("2 / span 2");
  });

  it("moves an item's start column with the window", () => {
    resizeTo(breakpoints.sm - 1);
    render(
      <LoomarrProvider theme="dark">
        <Grid columns={{ base: 1, sm: ["auto", "1fr"] }}>
          <GridItem data-testid="rest" from={{ base: 1, sm: 2 }} span="full">
            rest
          </GridItem>
        </Grid>
      </LoomarrProvider>,
    );
    expect(screen.getByTestId("rest").style.gridColumn).toBe("1 / -1");

    resizeTo(breakpoints.sm);
    expect(screen.getByTestId("rest").style.gridColumn).toBe("2 / -1");
  });

  it("never spans past the columns that exist", () => {
    render(
      <LoomarrProvider theme="dark">
        <Grid columns={2}>
          <GridItem data-testid="item" span={5}>
            x
          </GridItem>
        </Grid>
      </LoomarrProvider>,
    );
    expect(screen.getByTestId("item").style.gridColumn).toBe("span 2 / span 2");
  });

  it("renders the element it is asked for, so a list keeps its structure", () => {
    render(
      <LoomarrProvider theme="dark">
        <Grid aria-label="grid" columns={2} render="ul">
          <li>a</li>
          <li>b</li>
        </Grid>
      </LoomarrProvider>,
    );
    expect(screen.getByRole("list", { name: "grid" }).tagName).toBe("UL");
  });

  it("aligns items within their row", () => {
    render(
      <LoomarrProvider theme="dark">
        <Grid align="center" aria-label="grid" columns={2} role="group" />
      </LoomarrProvider>,
    );
    expect(getComputedStyle(grid()).alignItems).toBe("center");
  });

  it("lets a caller's style win", () => {
    render(
      <LoomarrProvider theme="dark">
        <Grid aria-label="grid" columns={2} role="group" style={{ gridTemplateColumns: "auto 1fr" }} />
      </LoomarrProvider>,
    );
    expect(getComputedStyle(grid()).gridTemplateColumns).toBe("auto 1fr");
  });
});
