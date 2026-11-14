import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { Card, LoomarrProvider } from "../index";
import { declarationsOf, tagBefore } from "./support/atomic-css";

describe("Card", () => {
  it("uses the compact 12px radius, not Surface's own 16px default", () => {
    const markup = renderToStaticMarkup(
      <LoomarrProvider>
        <Card>content</Card>
      </LoomarrProvider>,
    );
    expect(markup).toMatch(/border-radius:var\(--t-radius-cardCompact\)/);
    expect(markup).not.toMatch(/border-radius:var\(--t-radius-card\)/);
  });

  // `border-<hue>/40 bg-<hue>/5`, in both themes: the brand hue is the same in each.
  it.each([
    ["caution", "245,217,10"],
    ["onair", "229,72,77"],
    ["signal", "255,176,32"],
    ["suggest", "214,64,159"],
    ["tune", "76,201,232"],
  ] as const)("draws the %s tone as a 40%% hairline over a 5%% wash", (tone, rgb) => {
    for (const theme of ["dark", "light"] as const) {
      const markup = renderToStaticMarkup(
        <LoomarrProvider theme={theme}>
          <Card tone={tone}>content</Card>
        </LoomarrProvider>,
      );
      const declarations = declarationsOf(markup, tagBefore(markup, "content"));
      expect(declarations, `${tone} border in ${theme}`).toContain(`border-color:rgba(${rgb},0.4)`);
      expect(declarations, `${tone} fill in ${theme}`).toContain(`background-color:rgba(${rgb},0.05)`);
    }
  });

  it("keeps the raised fill when it has no tone", () => {
    const markup = renderToStaticMarkup(
      <LoomarrProvider>
        <Card>content</Card>
      </LoomarrProvider>,
    );
    expect(declarationsOf(markup, tagBefore(markup, "content"))).not.toContain("rgba(");
  });
});
