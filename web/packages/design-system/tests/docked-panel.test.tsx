// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { DockedPanel, LoomarrProvider } from "../index";

// The web DockedPanel is a non-modal panel docked inside the player's frame. What it owes the
// keyboard is pinned by driving keys: Escape closes it and goes no further, and Tab walks straight
// out of it, because nothing traps focus. The native adapter is not exercised here.

afterEach(cleanup);

const renderPanel = (props: Partial<React.ComponentProps<typeof DockedPanel>> = {}) => {
  const onFrameKey = vi.fn();
  const onClose = props.onClose ?? vi.fn();
  const view = render(
    <LoomarrProvider>
      {/* biome-ignore lint/a11y/noStaticElementInteractions: stands in for the player frame's key handler */}
      <div data-testid="frame" onKeyDown={onFrameKey} style={{ position: "relative" }}>
        <button type="button">Before</button>
        <DockedPanel closeLabel="Close channels" title="Channels" {...props} onClose={onClose}>
          <button type="button">First row</button>
          <button type="button">Second row</button>
        </DockedPanel>
        <button type="button">After</button>
      </div>
    </LoomarrProvider>,
  );
  return { ...view, onClose, onFrameKey };
};

const focused = () =>
  document.activeElement?.textContent || document.activeElement?.getAttribute("aria-label");

describe("DockedPanel (web)", () => {
  it("is a dialog named by its title, docked inside the frame, and not modal", () => {
    renderPanel();
    const panel = screen.getByRole("dialog", { name: "Channels" });
    expect(panel.hasAttribute("aria-modal")).toBe(false);
    expect(screen.getByTestId("frame").contains(panel)).toBe(true);
    expect([panel.style.position, panel.style.right, panel.style.width]).toEqual([
      "absolute",
      "0px",
      "372px",
    ]);
  });

  it("docks to the left edge at a given width", () => {
    renderPanel({ edge: "left", width: 320 });
    const panel = screen.getByRole("dialog", { name: "Channels" });
    expect([panel.style.left, panel.style.right, panel.style.width]).toEqual(["0px", "", "320px"]);
  });

  it("closes on Escape from inside it, and the frame behind never sees the key", async () => {
    const { onClose, onFrameKey } = renderPanel();
    screen.getByRole("button", { name: "First row" }).focus();
    await userEvent.keyboard("{Escape}");
    expect(onClose).toHaveBeenCalledTimes(1);
    expect(onFrameKey).not.toHaveBeenCalled();
  });

  it("leaves other keys to the frame", async () => {
    const { onClose, onFrameKey } = renderPanel();
    screen.getByRole("button", { name: "First row" }).focus();
    await userEvent.keyboard("{ArrowDown}");
    expect(onClose).not.toHaveBeenCalled();
    expect(onFrameKey).toHaveBeenCalledTimes(1);
  });

  it("closes from its close button", async () => {
    const { onClose } = renderPanel();
    await userEvent.click(screen.getByRole("button", { name: "Close channels" }));
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("does not take focus when it opens", () => {
    renderPanel();
    expect(document.activeElement).toBe(document.body);
  });

  it("does not trap focus: Tab walks through it and out the far side, and back", async () => {
    renderPanel();
    const walk: (string | null | undefined)[] = [];
    for (let step = 0; step < 5; step++) {
      await userEvent.tab();
      walk.push(focused());
    }
    expect(walk).toEqual(["Before", "Close channels", "First row", "Second row", "After"]);
    await userEvent.tab({ shift: true });
    expect(focused()).toBe("Second row");
  });

  it("shows the meta text beside the title", () => {
    renderPanel({ meta: "now to next 2h" });
    expect(screen.getByText("now to next 2h").textContent).toBe("now to next 2h");
  });
});
