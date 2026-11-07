import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { Card, LoomarrProvider } from "../index";

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
});
