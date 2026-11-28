import type { LineupGroupKind } from "@loomarr/core/channel-lineup";
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { render } from "@/test/render";
import { WallTabs } from "./wall-tabs";

const KINDS: LineupGroupKind[] = ["favourites", "all", "recent", "decade"];

const mount = (group: LineupGroupKind = "all") => {
  const onChoose = vi.fn();
  render(<WallTabs group={group} kinds={KINDS} onChoose={onChoose} />);
  return onChoose;
};
const tab = (name: string) => screen.getByRole("tab", { name });

afterEach(() => vi.restoreAllMocks());

describe("WallTabs", () => {
  it("shows the four groups in order with the active one selected, and no find", () => {
    mount("recent");
    expect(screen.getAllByRole("tab").map((t) => t.getAttribute("aria-label"))).toEqual([
      "Favourites",
      "All",
      "Recent",
      "By decade",
    ]);
    expect(screen.getByRole("tab", { selected: true })).toBe(tab("Recent"));
    expect(screen.queryByRole("button", { name: /find/i })).not.toBeInTheDocument();
  });

  it("is one tab stop, with Left and Right moving inside and stopping at the ends", async () => {
    const onChoose = mount();
    await userEvent.tab();
    expect(tab("All")).toHaveFocus();
    await userEvent.keyboard("{ArrowLeft}{ArrowLeft}");
    expect(tab("Favourites")).toHaveFocus();
    await userEvent.keyboard("{ArrowRight}{ArrowRight}{ArrowRight}{ArrowRight}");
    expect(tab("By decade")).toHaveFocus();
    expect(screen.getAllByRole("tab").filter((t) => t.tabIndex === 0)).toEqual([tab("By decade")]);
    expect(onChoose).not.toHaveBeenCalled();
    await userEvent.tab();
    expect(document.body).toHaveFocus();
  });

  it("calls back with the group chosen by Enter or a press", async () => {
    const onChoose = mount();
    await userEvent.tab();
    await userEvent.keyboard("{ArrowRight}{Enter}");
    expect(onChoose).toHaveBeenLastCalledWith("recent");
    await userEvent.click(tab("By decade"));
    expect(onChoose).toHaveBeenLastCalledWith("decade");
  });

  it("draws no ring and takes no focus for a pointer press, and rings the keys", async () => {
    const onChoose = mount();
    await userEvent.click(tab("Favourites"));
    expect(onChoose).toHaveBeenCalledWith("favourites");
    expect(document.body).toHaveFocus();
    expect(screen.queryByTestId("ring-actionFocus")).not.toBeInTheDocument();
    // jsdom does not know which focus came from a key, which a browser answers here.
    const native = Element.prototype.matches;
    vi.spyOn(Element.prototype, "matches").mockImplementation(
      new Proxy(native, {
        apply: (original, element, args: [string]) =>
          args[0] === ":focus-visible" || Reflect.apply(original, element, args),
      }),
    );
    await userEvent.tab();
    await userEvent.keyboard("{ArrowRight}");
    expect(within(tab("Recent")).getByTestId("ring-actionFocus")).toBeInTheDocument();
    expect(screen.getAllByTestId("ring-actionFocus")).toHaveLength(1);
  });
});
