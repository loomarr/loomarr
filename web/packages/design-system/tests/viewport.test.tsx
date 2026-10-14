import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { LoomarrProvider, resolveViewportInsets, Screen, TV_OVERSCAN_INSET } from "../index";

describe("viewport-safe layout", () => {
  it("keeps TV content inside the established overscan margin", () => {
    expect(resolveViewportInsets("tv")).toEqual({
      bottom: TV_OVERSCAN_INSET,
      left: TV_OVERSCAN_INSET,
      right: TV_OVERSCAN_INSET,
      top: TV_OVERSCAN_INSET,
    });
  });

  it("adds touch gutters inside real device safe-area insets", () => {
    expect(resolveViewportInsets("touch", { bottom: 34, left: 0, right: 0, top: 47 })).toEqual({
      bottom: 58,
      left: 24,
      right: 24,
      top: 71,
    });
  });

  it("applies provider insets through the Screen root interface", () => {
    const markup = renderToStaticMarkup(
      <LoomarrProvider insets={{ bottom: 34, left: 0, right: 0, top: 47 }}>
        <Screen density="touch">Safe content</Screen>
      </LoomarrProvider>,
    );

    expect(markup).toContain("Safe content");
    expect(markup).toContain("min-height:100vh");
    expect(markup).toContain("padding-top:71px");
    expect(markup).toContain("padding-bottom:58px");
  });
});
