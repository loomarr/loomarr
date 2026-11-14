// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

// Native has no CSS grid, so Grid lays the cells out as rows of flex items. This runs the component
// as native does (not the web), to hold that layout to the same cells the web's grid draws.
vi.mock("@tamagui/core", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tamagui/core")>()),
  isWeb: false,
}));

import { Grid, GridItem, LoomarrProvider } from "../index";

afterEach(cleanup);

const cell = (text: string) => screen.getByText(text).parentElement as HTMLElement;
const rowOf = (text: string) => cell(text).parentElement as HTMLElement;

describe("Grid without CSS grid (native)", () => {
  it("fills rows of equal columns and starts a new row when one is full", () => {
    render(
      <LoomarrProvider theme="dark">
        <Grid columns={2} gap="3">
          <span>a</span>
          <span>b</span>
          <span>c</span>
        </Grid>
      </LoomarrProvider>,
    );

    expect(rowOf("a")).toBe(rowOf("b"));
    expect(rowOf("c")).not.toBe(rowOf("a"));
  });

  it("pads a short last row so its cell keeps the column width", () => {
    render(
      <LoomarrProvider theme="dark">
        <Grid columns={3}>
          <span>a</span>
          <span>b</span>
          <span>c</span>
          <span>d</span>
        </Grid>
      </LoomarrProvider>,
    );

    // Row two holds "d" and two columns of padding: a third of the row, not all of it.
    expect(rowOf("d").children).toHaveLength(2);
    expect(getComputedStyle(cell("d")).flexGrow).toBe("1");
    expect(getComputedStyle(rowOf("d").children[1] as HTMLElement).flexGrow).toBe("2");
  });

  it("gives an item the columns it spans, on a row of its own when it does not fit", () => {
    render(
      <LoomarrProvider theme="dark">
        <Grid columns={2}>
          <span>a</span>
          <GridItem span="full">
            <span>wide</span>
          </GridItem>
          <span>b</span>
        </Grid>
      </LoomarrProvider>,
    );

    expect(rowOf("a")).not.toBe(rowOf("wide"));
    expect(getComputedStyle(cell("wide")).flexGrow).toBe("2");
    expect(rowOf("b")).not.toBe(rowOf("wide"));
  });
});
