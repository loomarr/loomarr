// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import { LoomarrProvider, VisuallyHidden } from "../index";

afterEach(cleanup);

describe("VisuallyHidden", () => {
  it("keeps its words in the accessibility tree, clipped to a pixel", () => {
    render(
      <LoomarrProvider theme="dark">
        <VisuallyHidden>Skip the intro</VisuallyHidden>
      </LoomarrProvider>,
    );

    const hidden = screen.getByText("Skip the intro");
    expect(hidden.tagName).toBe("DIV");
    expect(hidden.style.position).toBe("absolute");
    expect(hidden.style.width).toBe("1px");
    expect(hidden.style.height).toBe("1px");
    expect(hidden.style.overflow).toBe("hidden");
    expect(hidden.style.display).not.toBe("none");
  });

  it("draws the element it is told to, so a table caption stays a caption", () => {
    render(
      <LoomarrProvider theme="dark">
        <table>
          <VisuallyHidden as="caption">Schedule</VisuallyHidden>
        </table>
      </LoomarrProvider>,
    );

    expect(screen.getByText("Schedule").tagName).toBe("CAPTION");
  });
});
