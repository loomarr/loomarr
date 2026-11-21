import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { LoomarrProvider, Stack } from "../index";

const render = (node: React.ReactNode) =>
  renderToStaticMarkup(<LoomarrProvider theme="dark">{node}</LoomarrProvider>);

const hairline = "border-top:1px solid var(--borderDecorative)";

describe("Stack divided", () => {
  it("draws a hairline on every DOM child but the first", () => {
    const markup = render(
      <Stack divided render="ul">
        <li>one</li>
        <li>two</li>
        <li>three</li>
      </Stack>,
    );

    expect(markup.split(hairline)).toHaveLength(3);
    expect(markup).toContain("<li>one</li>");
    expect(markup).toContain(`<li style="${hairline}">two</li>`);
  });

  it("keeps a child's own style beside the hairline", () => {
    const markup = render(
      <Stack divided>
        <li>one</li>
        <li style={{ color: "red" }}>two</li>
      </Stack>,
    );

    expect(markup).toContain(`style="color:red;${hairline}"`);
  });

  it("leaves no line for a child that renders nothing", () => {
    const markup = render(
      <Stack divided>
        {false}
        <li>one</li>
        {null}
        <li>two</li>
      </Stack>,
    );

    expect(markup.split(hairline)).toHaveLength(2);
    expect(markup).toContain("<li>one</li>");
  });

  it("draws the line on a Stack child as the decorative border", () => {
    const markup = render(
      <Stack divided>
        <Stack>one</Stack>
        <Stack>two</Stack>
      </Stack>,
    );

    expect(markup).toContain("border-top-width:1px");
    expect(markup.split("border-top-width:1px")).toHaveLength(2);
    expect(markup).toContain("border-top-color:var(--borderDecorative)");
  });

  it("draws the line on the leading edge of a row", () => {
    const markup = render(
      <Stack direction="row" divided>
        <span>a</span>
        <span>b</span>
      </Stack>,
    );

    expect(markup).toContain("border-left:1px solid var(--borderDecorative)");
    expect(markup).not.toContain("border-top:");
  });

  it("draws nothing between children by default", () => {
    const markup = render(
      <Stack>
        <li>one</li>
        <li>two</li>
      </Stack>,
    );

    expect(markup).toContain("<li>one</li><li>two</li>");
  });
});
