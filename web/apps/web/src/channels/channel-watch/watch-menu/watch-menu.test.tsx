import { LoomarrProvider } from "@loomarr/design-system";
import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { watchMenu } from "./watch-menu";
import type { WatchMenuOptions } from "./watch-menu.type";

const channel = { id: "ch-4", name: "Night Owl", number: 4, playable: true };

const options = (slot: Partial<WatchMenuOptions["slot"]>, over: Partial<WatchMenuOptions> = {}): WatchMenuOptions => ({
  audio: [],
  density: "pointer",
  detailRows: [],
  favourite: { on: false, onPress: vi.fn() },
  goLive: undefined,
  onCopyDetails: vi.fn(),
  onLastChannel: vi.fn(),
  onQuality: vi.fn(),
  onSubtitles: vi.fn(),
  qualityOptions: [
    { label: "Auto · 1080p", selected: true, value: "auto" },
    { label: "720p", selected: false, value: "720" },
  ],
  slot: {
    close: vi.fn(),
    lastChannel: undefined,
    name: "more",
    openPage: vi.fn(),
    page: undefined,
    ...slot,
  },
  startOver: undefined,
  subtitleOptions: [{ label: "Off", selected: true, value: "off" }],
  switchSound: { on: true, onChange: vi.fn() },
  video: { current: null },
  ...over,
});

const draw = (menu: WatchMenuOptions) =>
  render(<LoomarrProvider theme="dark">{watchMenu(menu)}</LoomarrProvider>);

describe("watchMenu", () => {
  it("draws More's rows, with Picture and Playback details opening its inner pages", () => {
    const menu = options({});
    draw(menu);
    const more = screen.getByRole("dialog", { name: "More" });
    fireEvent.click(within(more).getByRole("button", { name: /^Picture/ }));
    expect(menu.slot.openPage).toHaveBeenCalledWith("picture");
    fireEvent.click(within(more).getByRole("button", { name: "Playback details" }));
    expect(menu.slot.openPage).toHaveBeenCalledWith("details");
  });

  it("draws the Picture page with the way back to More, and a pick goes to the player", () => {
    const menu = options({ page: "picture" });
    draw(menu);
    fireEvent.click(screen.getByRole("radio", { name: "720p" }));
    expect(menu.onQuality).toHaveBeenCalledWith("720");
    fireEvent.click(screen.getByRole("button", { name: "Back from Picture" }));
    expect(menu.slot.openPage).toHaveBeenCalledWith(undefined);
  });

  it("tunes Last channel and closes, and draws it unavailable when it cannot be played", () => {
    const menu = options({ lastChannel: { channel, id: "ch-4", playable: true } });
    const { unmount } = draw(menu);
    fireEvent.click(screen.getByRole("button", { name: /^Last channel/ }));
    expect(menu.slot.close).toHaveBeenCalledOnce();
    expect(menu.onLastChannel).toHaveBeenCalledWith("ch-4");
    unmount();
    draw(options({ lastChannel: { channel, id: "ch-4", playable: false } }));
    expect(screen.getByRole("button", { name: /^Last channel/ }).getAttribute("aria-disabled")).toBe("true");
  });

  it("names the favourite row for the watched channel's star", () => {
    draw(options({}, { favourite: { on: true, onPress: vi.fn() } }));
    expect(screen.getByRole("button", { name: "Remove from Favourites" })).toBeTruthy();
  });

  it("draws Subtitles and the shortcuts sheet as their own menus", () => {
    const { unmount } = draw(options({ name: "subtitles" }));
    expect(within(screen.getByRole("dialog", { name: "Subtitles" })).getByRole("radio", { name: "Off" })).toBeTruthy();
    unmount();
    draw(options({ name: "shortcuts" }));
    expect(within(screen.getByRole("dialog", { name: "Keyboard shortcuts" })).getByText("Last channel")).toBeTruthy();
  });
});
