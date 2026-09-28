import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
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
    expect(star.parentElement?.closest("button")).toBeNull();
    await userEvent.click(star);
    expect(onFavourite).toHaveBeenCalledWith("b", false);
  });

  it("closes on Escape and from its close button", async () => {
    const onClose = vi.fn();
    renderDrawer({ onClose });
    await userEvent.keyboard("{Escape}");
    await userEvent.click(screen.getByRole("button", { name: "Close channels" }));
    expect(onClose).toHaveBeenCalledTimes(2);
  });
});
