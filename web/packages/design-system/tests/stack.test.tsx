import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { LoomarrProvider, Stack } from "../index";

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
