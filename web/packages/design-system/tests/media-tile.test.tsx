// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createRef } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { LoomarrProvider, MediaTile, Tooltip } from "../index";

afterEach(cleanup);

const inTheme = (ui: React.ReactNode) => <LoomarrProvider theme="dark">{ui}</LoomarrProvider>;

const tile = (props: Partial<React.ComponentProps<typeof MediaTile>> = {}) => (
  <MediaTile height={64} label="Preview Cereal ad, 1989" onPress={() => undefined} width={112} {...props}>
    <img alt="" src="data:image/gif;base64,R0lGODlhAQABAAAAACw=" />
  </MediaTile>
);

describe("MediaTile", () => {
  it("is one button named by its label, holding the picture", () => {
    render(inTheme(tile()));

    const button = screen.getByRole("button", { name: "Preview Cereal ad, 1989" });
    expect(button.tagName).toBe("BUTTON");
    expect(button.getAttribute("type")).toBe("button");
    expect(button.querySelector("img")).not.toBeNull();
    expect(button.style.width).toBe("112px");
    expect(button.style.height).toBe("64px");
    expect(button.style.overflow).toBe("hidden");
  });

  it("presses with the mouse and with Enter and Space", async () => {
    const onPress = vi.fn();
    const user = userEvent.setup();
    render(inTheme(tile({ onPress })));

    await user.click(screen.getByRole("button"));
    await user.keyboard("{Enter}");
    await user.keyboard(" ");

    expect(onPress).toHaveBeenCalledTimes(3);
  });

  it("is named by a marker elsewhere when the caller points at one", () => {
    render(
      inTheme(
        <>
          <span id="marker">Segment #3</span>
          {tile({ "aria-labelledby": "marker", label: undefined })}
        </>,
      ),
    );

    expect(screen.getByRole("button", { name: "Segment #3" })).toBeTruthy();
  });

  it("draws the focus ring only while focused, outside the edge", async () => {
    const user = userEvent.setup();
    render(inTheme(tile()));
    const button = screen.getByRole("button");
    expect(button.style.outline).toBe("none");

    await user.tab();

    expect(document.activeElement).toBe(button);
    expect(button.style.outline).toContain("2px solid");
    expect(button.style.outlineOffset).toBe("2px");
  });

  it("draws the focus ring inside the edge for a tile that fills a clipping frame", async () => {
    const user = userEvent.setup();
    render(inTheme(tile({ focusInset: true })));

    await user.tab();

    expect(screen.getByRole("button").style.outlineOffset).toBe("-2px");
  });

  it("marks the current tile with a neutral ring that focus does not replace", async () => {
    const user = userEvent.setup();
    render(inTheme(tile({ current: true })));
    const button = screen.getByRole("button");
    expect(button.style.boxShadow).toContain("0 0 0 2px");

    await user.tab();

    expect(button.style.boxShadow).toContain("0 0 0 2px");
    expect(button.style.outline).toContain("2px solid");
  });

  it("does not press when disabled", async () => {
    const onPress = vi.fn();
    const user = userEvent.setup();
    render(inTheme(tile({ disabled: true, onPress })));
    const button = screen.getByRole("button") as HTMLButtonElement;

    await user.click(button);

    expect(onPress).not.toHaveBeenCalled();
    expect(button.disabled).toBe(true);
  });

  it("hands its element to a ref and to the press, for a caller that returns focus to it", async () => {
    const ref = createRef<HTMLButtonElement>();
    // React clears `currentTarget` once the handler returns, so the element is read inside it.
    let pressed: unknown;
    const onPress = vi.fn((event: { currentTarget: unknown }) => {
      pressed = event.currentTarget;
    });
    render(
      inTheme(
        <MediaTile label="Open" onPress={onPress} ref={ref}>
          <span>x</span>
        </MediaTile>,
      ),
    );

    expect(ref.current?.tagName).toBe("BUTTON");
    await userEvent.click(screen.getByRole("button", { name: "Open" }));
    expect(pressed).toBe(ref.current);
  });

  it("is a Tooltip's trigger: hover opens it, and the tile keeps its name and its press", async () => {
    const onPress = vi.fn();
    render(
      inTheme(
        <Tooltip content="Click to play this exact clip." id="tile-tip">
          {tile({ "aria-describedby": "tile-tip", onPress })}
        </Tooltip>,
      ),
    );
    const button = screen.getByRole("button", { name: "Preview Cereal ad, 1989" });

    await userEvent.hover(button);
    expect((await screen.findByRole("tooltip")).textContent).toBe("Click to play this exact clip.");

    await userEvent.click(button);
    expect(onPress).toHaveBeenCalledTimes(1);
  });

  it("opens a Tooltip from keyboard focus", async () => {
    render(inTheme(<Tooltip content="Preview this segment">{tile()}</Tooltip>));

    await userEvent.tab();

    expect((await screen.findByRole("tooltip")).textContent).toBe("Preview this segment");
  });
});
