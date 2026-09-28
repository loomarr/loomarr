import type { GuideAiring } from "@loomarr/api/models/guideAiring";
import type { GuideChannelTimeline } from "@loomarr/api/models/guideChannelTimeline";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { ChannelDrawer } from "./channel-drawer";
import type { DrawerChannel } from "./channel-drawer.type";
import { drawerChannels } from "./drawer-channels";

const NOW = Date.UTC(2026, 8, 28, 21, 30);
const MIN = 60_000;

const airing = (
  id: string,
  startMin: number,
  stopMin: number,
  extra: Partial<GuideAiring> = {},
): GuideAiring => ({
  kind: "program",
  scheduleBlockId: id,
  startMs: NOW + startMin * MIN,
  stopMs: NOW + stopMin * MIN,
  title: id,
  ...extra,
});

const timeline = (extra: Partial<GuideChannelTimeline>): GuideChannelTimeline => ({
  airings: [],
  channelId: "c",
  name: "A channel",
  number: 1,
  pendingCount: 0,
  status: "live",
  ...extra,
});

describe("drawerChannels", () => {
  it("reads what's on now, the minutes left and the strip from just before now to two hours on", () => {
    const [first] = drawerChannels(
      [
        timeline({
          airings: [
            airing("before", -120, -60),
            airing("now", -30, 20, { series: "A sitcom", title: "The pilot" }),
            airing("break", 20, 23, { kind: "filler" }),
            airing("later", 23, 180),
          ],
        }),
      ],
      NOW,
    );
    expect(first?.now).toBe("A sitcom — “The pilot”");
    expect(first?.minutesLeft).toBe(20);
    expect(first?.blocks.map((b) => [b.key, b.label, b.minutes, b.pod])).toEqual([
      ["now", "A sitcom", 44, false],
      ["break", undefined, 3, true],
      ["later", "later", 97, false],
    ]);
  });

  it("says why a paused or unfinished channel has nothing on, in channel-number order", () => {
    const rows = drawerChannels(
      [
        timeline({ channelId: "b", number: 9, status: "building", airings: [airing("x", -5, 5)] }),
        timeline({ channelId: "p", number: 3, status: "paused", airings: [airing("y", -5, 5)] }),
      ],
      NOW,
    );
    expect(rows.map((r) => [r.id, r.offAir, r.now, r.blocks.length])).toEqual([
      ["p", "paused", undefined, 0],
      ["b", "building", undefined, 0],
    ]);
  });
});

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
