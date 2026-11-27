import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { render } from "@/test/render";
import { ChannelDrawer } from "./channel-drawer";
import type { DrawerChannel } from "./channel-drawer.type";

const row = (
  id: string,
  number: number,
  name: string,
  extra: Partial<DrawerChannel> = {},
): DrawerChannel => ({
  id,
  number,
  name,
  blocks: [],
  ...extra,
});

describe("ChannelDrawer", () => {
  const channels = [
    row("a", 3, "Westerns", { now: "A western", minutesLeft: 12 }),
    row("b", 7, "Cartoons", { now: "A cartoon", minutesLeft: 4 }),
    row("c", 12, "Nature", { offAir: "paused" }),
    row("d", 21, "Sitcoms"),
  ];
  const renderDrawer = (props: Partial<Parameters<typeof ChannelDrawer>[0]> = {}) =>
    render(
      <ChannelDrawer
        channels={channels}
        tunedId="a"
        favourites={["b"]}
        recent={["b", "c", "d", "a"]}
        nowPercent={17}
        onTune={vi.fn()}
        onFavourite={vi.fn()}
        onClose={vi.fn()}
        {...props}
      />,
    );

  const groupHeaders = () => screen.getAllByRole("heading", { level: 3 }).map((h) => h.textContent);
  const group = (name: string) => within(screen.getByRole("region", { name }));

  it("groups favourites, recents that aren't favourites (three at most), then every channel", () => {
    renderDrawer();
    const names = (name: string) =>
      group(name)
        .getAllByRole("button", { name: /^\d/ })
        .map((b) => b.textContent);
    expect(groupHeaders()).toEqual(["FAVORITES", "RECENT", "ALL CHANNELS"]);
    expect(names("FAVORITES")).toEqual([expect.stringContaining("Cartoons")]);
    expect(names("RECENT")).toHaveLength(3);
    expect(names("RECENT")[0]).toContain("Nature");
    expect(group("ALL CHANNELS").getByText("▸ off air")).toBeInTheDocument();
    expect(screen.getByText("4 channels · type a number to tune direct")).toBeInTheDocument();
  });

  it("marks the tuned channel", () => {
    renderDrawer();
    const tuned = group("ALL CHANNELS").getByRole("button", { current: true });
    expect(tuned).toHaveTextContent("Westerns");
    expect(tuned).toHaveTextContent("TUNED");
    // Only the tuned row is current; the tuned look itself is the design system's `selected` row.
    // (Westerns is listed under Recent as well as All channels.)
    for (const current of screen.getAllByRole("button", { current: true })) {
      expect(current).toHaveTextContent("Westerns");
    }
  });

  it("finds by name, programme or number", async () => {
    renderDrawer();
    await userEvent.type(screen.getByRole("searchbox"), "cartoon");
    expect(groupHeaders()).toEqual(["MATCHES"]);
    expect(screen.getByText("1 of 4 channels · guide has the full week")).toBeInTheDocument();
    await userEvent.clear(screen.getByRole("searchbox"));
    await userEvent.type(screen.getByRole("searchbox"), "21");
    expect(group("MATCHES").getByText("Sitcoms")).toBeInTheDocument();
  });

  it("stars and unstars from a button beside the row, not inside it", async () => {
    const onFavourite = vi.fn();
    renderDrawer({ onFavourite });
    const star = screen.getAllByRole("button", { name: "Favorite Cartoons" })[0] as HTMLElement;
    expect(star).toHaveAttribute("aria-pressed", "true");
    expect(star.parentElement?.closest('button, [role="button"]')).toBeNull();
    await userEvent.click(star);
    expect(onFavourite).toHaveBeenCalledWith("b", false);
  });

  it("is a non-modal dialog named Channels, so the player's arrow keys leave its list alone", () => {
    renderDrawer();
    const dialog = screen.getByRole("dialog", { name: "Channels" });
    expect(dialog.hasAttribute("aria-modal")).toBe(false);
  });

  it("moves focus into the find box, then tabs through each row followed by its star and out", async () => {
    renderDrawer({ favourites: [], recent: [] });
    expect(screen.getByRole("searchbox")).toHaveFocus();
    const order: string[] = [];
    for (let i = 0; i < 9; i++) {
      await userEvent.tab();
      order.push(document.activeElement?.getAttribute("aria-label") ?? "");
    }
    expect(order).toEqual([
      "03 Westerns, tuned, A western, 12m left",
      "Favorite Westerns",
      "07 Cartoons, A cartoon, 4m left",
      "Favorite Cartoons",
      "12 Nature, off air",
      "Favorite Nature",
      "21 Sitcoms",
      "Favorite Sitcoms",
      "",
    ]);
  });

  it("tunes from a row with Enter or Space and closes on Escape pressed on it", async () => {
    const onTune = vi.fn();
    const onClose = vi.fn();
    renderDrawer({ favourites: [], onClose, onTune, recent: [] });
    const cartoons = screen.getByRole("button", { name: /^07 Cartoons/ });
    cartoons.focus();
    await userEvent.keyboard("{Enter}");
    await userEvent.keyboard(" ");
    expect(onTune).toHaveBeenNthCalledWith(1, "b");
    expect(onTune).toHaveBeenNthCalledWith(2, "b");
    await userEvent.keyboard("{Escape}");
    expect(onClose).toHaveBeenCalledOnce();
  });

  // WCAG 2.5.3, label in name, as axe checks it: the row's visible text, its text nodes joined as
  // they are with nothing between them, split into words, must run unbroken through the name's words.
  describe("a row's name holds every word the row shows", () => {
    const words = (text: string) =>
      text
        .toLowerCase()
        .split(/[^\p{L}\p{N}]+/u)
        .filter(Boolean);
    const visibleText = (row: HTMLElement) => {
      const walker = document.createTreeWalker(row, NodeFilter.SHOW_TEXT);
      let text = "";
      for (let node = walker.nextNode(); node; node = walker.nextNode()) text += node.nodeValue ?? "";
      return text;
    };
    const holdsInOrder = (name: string, shown: string) => {
      const have = words(name);
      const want = words(shown);
      return have.some((_, start) => want.every((word, i) => have[start + i] === word));
    };

    it("for a tuned row with a schedule strip, what is on and the time left", () => {
      renderDrawer({
        channels: [
          row("a", 3, "Westerns", {
            now: "A western",
            minutesLeft: 12,
            blocks: [
              { key: "1", label: "High Noon Hour", minutes: 60, pod: false },
              { key: "2", minutes: 5, pod: true },
              { key: "3", label: "Dusty Trails", minutes: 55, pod: false },
            ],
          }),
        ],
        favourites: [],
        recent: [],
        tunedId: "a",
      });
      const rowButton = screen.getAllByRole("button", { current: true })[0] as HTMLElement;
      const name = rowButton.getAttribute("aria-label") ?? "";
      expect(name).toBe("03 Westerns, tuned, High Noon Hour, Dusty Trails, A western, 12m left");
      expect(holdsInOrder(name, visibleText(rowButton))).toBe(true);
    });

    it("for an off-air row, which has no time left", () => {
      renderDrawer({ tunedId: "a" });
      const rowButton = screen.getAllByRole("button", { name: /^12 Nature/ })[0] as HTMLElement;
      expect(holdsInOrder(rowButton.getAttribute("aria-label") ?? "", visibleText(rowButton))).toBe(true);
    });
  });

  it("closes on Escape and from its close button", async () => {
    const onClose = vi.fn();
    renderDrawer({ onClose });
    await userEvent.keyboard("{Escape}");
    await userEvent.click(screen.getByRole("button", { name: "Close channels" }));
    expect(onClose).toHaveBeenCalledTimes(2);
  });
});
