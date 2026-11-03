import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import {
  LoomarrProvider,
  PHONE_GUTTER,
  resolveViewportInsets,
  resolveViewportScale,
  Screen,
  TV_OVERSCAN_INSET,
  ViewportStage,
} from "../index";

describe("viewport-safe layout", () => {
  it("keeps TV content inside the established overscan margin", () => {
    expect(resolveViewportInsets("tv")).toEqual({
      bottom: TV_OVERSCAN_INSET,
      left: TV_OVERSCAN_INSET,
      right: TV_OVERSCAN_INSET,
      top: TV_OVERSCAN_INSET,
    });
  });

  it("adds the phone gutter inside real device safe-area insets", () => {
    expect(resolveViewportInsets("touch", { bottom: 34, left: 0, right: 0, top: 47 })).toEqual({
      bottom: 34 + PHONE_GUTTER,
      left: PHONE_GUTTER,
      right: PHONE_GUTTER,
      top: 47 + PHONE_GUTTER,
    });
    // Every phone frame of the native mock (5d–5g) sets its content 16 pt from the edge.
    expect(PHONE_GUTTER).toBe(16);
  });

  // tvOS reports a 1080p screen as 1920 points wide at 1×; Android TV reports 960 dp at 2×, and the
  // tv density is drawn for that (#2141).
  it("lays Apple TV out at Android TV's width, and nothing else", () => {
    expect(resolveViewportScale(1920, true)).toBe(2);
    expect(resolveViewportScale(960, false)).toBe(1);
    expect(resolveViewportScale(1920, false)).toBe(1);
  });

  it("renders its children as they are off Apple TV", () => {
    const markup = renderToStaticMarkup(
      <ViewportStage>
        <span>Picture</span>
      </ViewportStage>,
    );

    expect(markup).toBe("<span>Picture</span>");
  });

  it("applies provider insets through the Screen root interface", () => {
    const markup = renderToStaticMarkup(
      <LoomarrProvider insets={{ bottom: 34, left: 0, right: 0, top: 47 }}>
        <Screen density="touch">Safe content</Screen>
      </LoomarrProvider>,
    );

    expect(markup).toContain("Safe content");
    expect(markup).toContain("min-height:100vh");
    expect(markup).toContain("padding-top:63px");
    expect(markup).toContain("padding-bottom:50px");
  });
});
