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
