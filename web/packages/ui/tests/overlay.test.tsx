import { LoomarrProvider } from "@loomarr/design-system";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import { TransientOverlay } from "../index";

describe("shared overlay composition", () => {
  it("renders non-modal playback feedback edge-to-edge with safe content padding", () => {
    const markup = renderToStaticMarkup(
      <LoomarrProvider insets={{ bottom: 20, left: 4, right: 6, top: 8 }}>
        <TransientOverlay
          density="touch"
          description="The Simpsons · Marge vs. the Monorail"
          onDismiss={vi.fn()}
          reducedMotion
          title="Now playing"
          visible
        />
      </LoomarrProvider>,
    );

    expect(markup).toContain("Now playing");
    expect(markup).toContain("The Simpsons · Marge vs. the Monorail");
    expect(markup).toContain("left:0");
    expect(markup).toContain("right:0");
    expect(markup).toContain("bottom:0");
    expect(markup).toContain("padding-left:28px");
    expect(markup).toContain("padding-right:30px");
    expect(markup).toContain("padding-bottom:44px");
  });
});
