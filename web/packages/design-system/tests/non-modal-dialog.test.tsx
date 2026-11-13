// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LoomarrProvider, NonModalDialog } from "../index";

// The web NonModalDialog is a named dialog that sits in the page. What it owes the keyboard is
// pinned by driving keys: it takes no focus, traps none, and leaves every key to the frame behind it.

afterEach(cleanup);

const renderLayer = () => {
  const onFrameKey = vi.fn();
  render(
    <LoomarrProvider>
      {/* biome-ignore lint/a11y/noStaticElementInteractions: stands in for the player frame's key handler */}
      <div onKeyDown={onFrameKey}>
        <button type="button">Before</button>
        <NonModalDialog
          label="Subtitles"
          style={{ position: "absolute", right: 0, width: 280 }}
          testID="layer"
        >
          <button type="button">Off</button>
          <button type="button">English</button>
        </NonModalDialog>
        <button type="button">After</button>
      </div>
    </LoomarrProvider>,
  );
  return { onFrameKey };
};

const focused = () => document.activeElement?.textContent;

describe("NonModalDialog (web)", () => {
  it("is a dialog named by its label, that is not modal, and wears the caller's placement", () => {
    renderLayer();
    const layer = screen.getByRole("dialog", { name: "Subtitles" });
    expect(layer.hasAttribute("aria-modal")).toBe(false);
    expect(layer.getAttribute("data-testid")).toBe("layer");
    expect(layer.contains(screen.getByRole("button", { name: "English" }))).toBe(true);
    expect(getComputedStyle(layer).width).toBe("280px");
  });

  it("does not take focus when it opens", () => {
    renderLayer();
    expect(document.activeElement).toBe(document.body);
  });

  it("does not trap focus: Tab walks through it and out the far side, and back", async () => {
    renderLayer();
    const walk: (string | null | undefined)[] = [];
    for (let step = 0; step < 4; step++) {
      await userEvent.tab();
      walk.push(focused());
    }
    expect(walk).toEqual(["Before", "Off", "English", "After"]);
    await userEvent.tab({ shift: true });
    expect(focused()).toBe("English");
  });

  it("leaves Escape and the arrows to the frame behind it", async () => {
    const { onFrameKey } = renderLayer();
    screen.getByRole("button", { name: "Off" }).focus();
    await userEvent.keyboard("{Escape}{ArrowDown}");
    expect(onFrameKey).toHaveBeenCalledTimes(2);
  });
});
