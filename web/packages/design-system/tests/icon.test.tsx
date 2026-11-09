import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { Icon, icons, LoomarrProvider } from "../index";

describe("Icon", () => {
  it("labels one image wrapper without leaking ARIA attributes onto SVG geometry", () => {
    const markup = renderToStaticMarkup(
      <LoomarrProvider>
        <Icon accessibilityLabel="Play" glyph={icons.play} />
      </LoomarrProvider>,
    );

    expect(markup).toContain('aria-label="Play"');
    expect(markup).toContain('role="img"');
    expect(markup).not.toMatch(/<(?:circle|line|path)[^>]*aria-label=/);
  });

  it("draws the Requests inbox glyph as a labelled image", () => {
    const markup = renderToStaticMarkup(
      <LoomarrProvider>
        <Icon accessibilityLabel="Requests" glyph={icons.requests} />
      </LoomarrProvider>,
    );

    expect(markup).toContain('aria-label="Requests"');
    expect(markup).toContain("<svg");
  });

  it("draws the catalog grid glyph as a labelled image", () => {
    const markup = renderToStaticMarkup(
      <LoomarrProvider>
        <Icon accessibilityLabel="Grid" glyph={icons.grid} />
      </LoomarrProvider>,
    );

    expect(markup).toContain('aria-label="Grid"');
    expect(markup).toContain("<svg");
  });

  it("draws the AI accent for the suggest tone", () => {
    const markup = renderToStaticMarkup(
      <LoomarrProvider>
        <Icon decorative glyph={icons.suggest} tone="suggest" />
        <Icon decorative glyph={icons.suggest} tone="primary" />
      </LoomarrProvider>,
    );
    const strokes = [...markup.matchAll(/stroke="([^"]+)"/g)].map((match) => match[1]);

    expect(strokes.length).toBeGreaterThan(1);
    expect(new Set(strokes).size).toBeGreaterThan(1);
  });

  it("lets the inherit tone follow the surrounding text colour on the web", () => {
    const markup = renderToStaticMarkup(
      <LoomarrProvider>
        <Icon decorative glyph={icons.passed} tone="inherit" />
      </LoomarrProvider>,
    );

    expect(markup).toContain("currentColor");
  });

  it("keeps decorative glyphs out of the accessibility tree", () => {
    const markup = renderToStaticMarkup(
      <LoomarrProvider>
        <Icon decorative glyph={icons.play} />
      </LoomarrProvider>,
    );

    expect(markup).toContain('aria-hidden="true"');
    expect(markup).not.toContain("aria-label=");
  });
});
