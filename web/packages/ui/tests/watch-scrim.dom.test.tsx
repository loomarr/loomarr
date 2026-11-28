// @vitest-environment jsdom

import { cleanup, renderUi, screen } from "@loomarr/test-support";
import { afterEach, describe, expect, it } from "vitest";

import { WatchScrim } from "../index";

afterEach(cleanup);

describe("WatchScrim's corner", () => {
  it("stands in the top leading corner, full to the control's far edges and fading out past them", () => {
    renderUi(<WatchScrim box={{ height: 36, width: 100, x: 64, y: 28 }} edge="corner" />);
    const corner = screen.getByTestId("watch-scrim-corner");
    // Ink to the control's right (164) and on for most of that reach again (feather 131).
    expect(corner.style.width).toBe("295px");
    // Ink to the control's foot (64); the soften of 54 fades it out below.
    expect(corner.style.height).toBe("64px");
    expect(corner.style.top).toBe("0px");
    expect(corner.style.left).toBe("0px");
    expect(corner.getAttribute("aria-hidden")).toBe("true");
  });

  it("follows the control down where it stands clear of a logo", () => {
    renderUi(<WatchScrim box={{ height: 36, width: 100, x: 64, y: 200 }} edge="corner" />);
    expect(screen.getByTestId("watch-scrim-corner").style.height).toBe("236px");
  });

  it("draws nothing before the control has been measured", () => {
    renderUi(<WatchScrim edge="corner" />);
    expect(screen.queryByTestId("watch-scrim-corner")).toBeNull();
  });
});
