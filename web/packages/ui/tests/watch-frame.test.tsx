import { LoomarrProvider, Surface } from "@loomarr/design-system";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { WatchFrame } from "../index";

describe("WatchFrame", () => {
  it("is the page's main landmark, headed for assistive tech, around the player", () => {
    const markup = renderToStaticMarkup(
      <LoomarrProvider theme="dark">
        <WatchFrame title="Watch Retro Toons">
          <p>player</p>
        </WatchFrame>
      </LoomarrProvider>,
    );

    expect(markup).toContain('role="main"');
    expect(markup).toMatch(/<h1[^>]*>Watch Retro Toons<\/h1>/);
    expect(markup).toContain("<p>player</p>");
  });

  it("scrolls down only when it must and never sideways, so no scrollbar is drawn for nothing (#3405)", () => {
    const markup = renderToStaticMarkup(
      <LoomarrProvider theme="dark">
        <WatchFrame title="Watch Retro Toons">
          <p>player</p>
        </WatchFrame>
      </LoomarrProvider>,
    );
    const main = /<div[^>]*role="main"[^>]*>/.exec(markup)?.[0] ?? "";
    // The design system emits one hashed class per style, so the class `overflow: scroll` would
    // carry is read from a Surface that asks for it, and the frame must not have it.
    const scrolling = renderToStaticMarkup(
      <LoomarrProvider theme="dark">
        <Surface overflow="scroll" />
      </LoomarrProvider>,
    );
    const scrollClass = /class="[^"]*?(_o-\d+)[^"]*"/.exec(scrolling)?.[1];

    expect(scrollClass).toBeDefined();
    expect(main).toMatch(/_o-\d+/);
    expect(main).not.toContain(scrollClass);
  });
});
