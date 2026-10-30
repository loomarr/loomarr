import { LoomarrProvider } from "@loomarr/design-system";
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
});
