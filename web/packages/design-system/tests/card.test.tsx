import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { Card, CardContent, CardFooter, CardHeader, CardTitle, LoomarrProvider } from "../index";

describe("Card", () => {
  it("composes header, title, content and footer around a Surface", () => {
    const markup = renderToStaticMarkup(
      <LoomarrProvider>
        <Card>
          <CardHeader>
            <CardTitle>Title</CardTitle>
          </CardHeader>
          <CardContent>Body</CardContent>
          <CardFooter>Footer</CardFooter>
        </Card>
      </LoomarrProvider>,
    );
    expect(markup).toContain("Title");
    expect(markup).toContain("Body");
    expect(markup).toContain("Footer");
  });

  // The legacy shadcn Card used `rounded-lg`, this app's own 12px radius step (`$cardCompact`),
  // not Surface's own 16px default (`$card`) — a card that quietly grew rounder corners would be
  // the visual regression this primitive exists to avoid.
  it("uses the legacy 12px radius, not Surface's own 16px default", () => {
    const markup = renderToStaticMarkup(
      <LoomarrProvider>
        <Card>content</Card>
      </LoomarrProvider>,
    );
    expect(markup).toMatch(/border-top-left-radius:var\(--t-radius-cardCompact\)/);
    expect(markup).not.toMatch(/border-top-left-radius:var\(--t-radius-card\)/);
  });
});
