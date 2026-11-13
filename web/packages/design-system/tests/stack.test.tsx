import { getTokens } from "@tamagui/core";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { LoomarrProvider, Stack, type StackProps, spaceSteps } from "../index";
import { webSpace } from "../web-theme";

const render = (node: React.ReactNode) =>
  renderToStaticMarkup(<LoomarrProvider theme="dark">{node}</LoomarrProvider>);

describe("Stack", () => {
  it("stacks a column with no gap by default and stretches children", () => {
    const markup = render(
      <Stack>
        <span>one</span>
        <span>two</span>
      </Stack>,
    );

    expect(markup).toContain("flex-direction:column");
    expect(markup).toContain("align-items:stretch");
    expect(markup).toContain("flex-wrap:nowrap");
    // It gives way as a flex item, as the <div> it replaces does.
    expect(markup).toContain("flex-shrink:1");
    expect(markup).toContain("<span>one</span><span>two</span>");
  });

  it("lays out a wrapping row from the spacing tokens", () => {
    const markup = render(
      <Stack align="center" direction="row" gap="control" justify="between" wrap>
        <span>a</span>
      </Stack>,
    );

    expect(markup).toContain("flex-direction:row");
    expect(markup).toContain("align-items:center");
    expect(markup).toContain("justify-content:space-between");
    expect(markup).toContain("flex-wrap:wrap");
    // `control` is the 12px spacing token, read through its CSS variable.
    expect(markup).toContain("gap:var(--t-space-control)");
  });

  // A step of the 4px grid (§2.3), named as Tailwind names it, so `gap-4` moves over as `gap="4"`
  // with the same 16px. The half steps the app uses off the grid move over at their own size,
  // never rounded onto it.
  it("spaces by a step of the spacing scale, on the grid or a half step off it", () => {
    const step = (gap: StackProps["gap"]) =>
      render(
        <Stack gap={gap}>
          <span>a</span>
        </Stack>,
      );

    // Every space token the provider registered, by the CSS variable it is emitted as.
    const spaceByVariable = () =>
      new Map(Object.values(getTokens().space).map((token) => [String(token.variable), token.val]));
    for (const [gap, px] of [
      ["4", 16],
      ["1", 4],
      ["1.5", 6],
      ["0.5", 2],
      ["1.75", 7],
      ["2.5", 10],
    ] as const) {
      expect(spaceSteps[gap]).toBe(px);
      // The Stack's own gap class reads a space variable, and that variable is the step's size.
      const markup = step(gap);
      const gapClass = markup.match(/data-href="[^"]*\bt__(g-\d+)/)?.[1];
      const variable =
        gapClass && markup.match(new RegExp(`\\._${gapClass}\\s*\\{gap:var\\((--t-space-[^)]+)\\)`))?.[1];
      expect(variable, `gap="${gap}"`).toBeDefined();
      expect(spaceByVariable().get(`var(${variable})`), `gap="${gap}"`).toBe(px);
    }
  });

  // `gap-x-4 gap-y-1.5` moves over as `columnGap="4" rowGap="1.5"`: each axis its own token, a half
  // step kept at its own size, and an axis left out falls back to `gap`.
  it("spaces a wrapped row's lines and columns separately, each from the spacing tokens", () => {
    // The size this Stack's own `property` class ("cg" column-gap, "rg" row-gap, "g" gap) resolves
    // to: the class is named in the markup, its rule reads a space variable, the variable is a token.
    const spaceByVariable = () =>
      new Map(Object.values(getTokens().space).map((token) => [String(token.variable), token.val]));
    const size = (markup: string, shorthand: "cg" | "g" | "rg", property: string) => {
      const id = markup.match(new RegExp(`data-href="[^"]*\\bt__(${shorthand}-\\d+)`))?.[1];
      const rule = id && markup.match(new RegExp(`\\._${id}\\s*\\{${property}:([^;}]+)`))?.[1];
      if (rule === "0px" || rule === "0") return 0;
      const variable = rule && rule.match(/^var\((--t-space-[^)]+)\)$/)?.[1];
      return variable ? spaceByVariable().get(`var(${variable})`) : undefined;
    };

    const both = render(
      <Stack columnGap="4" direction="row" rowGap="1.5" wrap>
        <span>a</span>
      </Stack>,
    );
    expect(size(both, "cg", "column-gap")).toBe(16);
    expect(size(both, "rg", "row-gap")).toBe(6);

    const oneAxis = render(
      <Stack direction="row" gap="2" rowGap="none" wrap>
        <span>a</span>
      </Stack>,
    );
    expect(size(oneAxis, "cg", "column-gap")).toBe(8);
    expect(size(oneAxis, "rg", "row-gap")).toBe(0);

    // No axis named: the single `gap`, as before.
    const plain = render(
      <Stack gap="4">
        <span>a</span>
      </Stack>,
    );
    expect(size(plain, "g", "gap")).toBe(16);
    expect(plain).not.toMatch(/data-href="[^"]*\bt__(?:cg|rg)-/);
  });

  it("emits the grid once: the web theme's spacing is the scale's grid steps", () => {
    expect(webSpace).toEqual({ 1: 4, 2: 8, 3: 12, 4: 16, 5: 20, 6: 24, 7: 28, 8: 32 });
    for (const [key, px] of Object.entries(webSpace)) {
      expect(spaceSteps[key as keyof typeof spaceSteps]).toBe(px);
    }
  });

  it("keeps a caller's class, element and accessible name, so a call site can move over whole", () => {
    const markup = render(
      <Stack
        aria-label="What can play now"
        className="text-muted-foreground mt-2"
        gap="inline"
        render="section"
      >
        <span>x</span>
      </Stack>,
    );

    expect(markup).toMatch(/<section[^>]*aria-label="What can play now"/);
    expect(markup).toMatch(/<section[^>]*class="[^"]*text-muted-foreground mt-2/);
  });

  it("maps end alignment to the flex keywords and passes the accessibility role through", () => {
    const markup = render(
      <Stack align="end" justify="end" role="group">
        <span>x</span>
      </Stack>,
    );

    expect(markup).toContain("align-items:flex-end");
    expect(markup).toContain("justify-content:flex-end");
    expect(markup).toContain('role="group"');
  });
});
