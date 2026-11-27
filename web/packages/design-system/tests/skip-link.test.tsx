// @vitest-environment jsdom
import { act, cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";

import { LoomarrProvider, SkipLink } from "../index";

afterEach(cleanup);

const renderLink = () =>
  render(
    <LoomarrProvider>
      <SkipLink href="#main-content">Skip to content</SkipLink>
      <button type="button">Rail</button>
      <main id="main-content" tabIndex={-1}>
        Page
      </main>
    </LoomarrProvider>,
  );

describe("SkipLink", () => {
  it("is a link to the content, the first Tab stop, and hidden to the eye at rest", async () => {
    const user = userEvent.setup();
    renderLink();
    const link = screen.getByRole("link", { name: "Skip to content" });

    expect(link.getAttribute("href")).toBe("#main-content");
    // Clipped to a pixel, not display:none, so Tab and a screen reader still find it.
    expect(link.style.display).toBe("");
    expect(link.style.width).toBe("1px");
    expect(link.style.height).toBe("1px");
    expect(link.style.position).toBe("absolute");

    await user.tab();
    expect(document.activeElement).toBe(link);
  });

  it("reveals itself at the top left while it holds focus and hides again after", async () => {
    renderLink();
    const link = screen.getByRole("link", { name: "Skip to content" });

    await act(async () => link.focus());
    expect(link.style.top).toBe("8px");
    expect(link.style.left).toBe("8px");
    expect(link.style.zIndex).toBe("50");
    expect(link.style.width).toBe("");
    expect(link.style.background).not.toBe("");

    await act(async () => link.blur());
    expect(link.style.width).toBe("1px");
  });
});
