import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { render } from "@/test/render";
import { sheetFor } from "./sheet-for";
import type { SheetForOptions } from "./sheet-for.type";

// The parts sheetFor only hands through (the player, the chrome, the schedule) are stand-ins; what is
// under test is which sheet a key opens, where, and what its rows do.
const options = (overrides: Partial<SheetForOptions> = {}): SheetForOptions =>
  ({
    audio: [{ label: "English", selected: true, value: "en" }],
    chrome: { goLive: vi.fn(), seekTo: vi.fn() },
    closeSheet: vi.fn(),
    copyDetails: vi.fn(),
    density: "pointer",
    detailRows: [],
    drawer: undefined,
    hasAirings: false,
    info: undefined,
    live: { mode: "live", viewerTimeMs: 0 },
    osdChannel: { id: "ch-7", name: "Cartoons", number: 7 },
    placement: "menu",
    player: { setQuality: vi.fn(), tune: vi.fn() },
    playbackWindow: { endMs: 0, startMs: 0 },
    qualityOptions: undefined,
    segments: [],
    serverNowMs: 0,
    setDetailsOpen: vi.fn(),
    setSubtitlesKey: vi.fn(),
    setSwitchSoundOn: vi.fn(),
    sheet: undefined,
    startOverMs: undefined,
    subtitleOptions: [
      { label: "Off", selected: true, value: "off" },
      { label: "English", selected: false, value: "en" },
    ],
    switchSoundOn: false,
    toggleSheet: vi.fn(),
    touch: false,
    video: { current: null },
    viewerTimeMs: 0,
    ...overrides,
  }) as unknown as SheetForOptions;

describe("sheetFor", () => {
  it("draws nothing while nothing is open", () => {
    expect(sheetFor(options())).toBeUndefined();
  });

  it("opens Subtitles and hands a choice straight to the player", async () => {
    const opts = options({ sheet: "subtitles" });
    render(<>{sheetFor(opts)}</>);
    expect(screen.getByText("Subtitles")).toBeTruthy();
    await userEvent.click(screen.getByRole("radio", { name: "English" }));
    expect(opts.setSubtitlesKey).toHaveBeenCalledWith("en");
  });

  it("keeps Info out of a pointer's menu, where the film strip shows it", () => {
    const info = { title: "Cartoon Classics" } as unknown as SheetForOptions["info"];
    expect(sheetFor(options({ info, placement: "menu", sheet: "info" }))).toBeUndefined();
  });

  it("opens Channels only when there is a drawer to open", () => {
    expect(sheetFor(options({ sheet: "channels" }))).toBeUndefined();
  });

  it("sends More's Playback details to a sheet on a phone and to the overlay on a pointer", async () => {
    const pointer = options({ sheet: "more" });
    const { unmount } = render(<>{sheetFor(pointer)}</>);
    await userEvent.click(screen.getByText("Playback details"));
    expect(pointer.closeSheet).toHaveBeenCalled();
    expect(pointer.setDetailsOpen).toHaveBeenCalledWith(true);
    unmount();

    const phone = options({ placement: "bottom", sheet: "more", touch: true });
    render(<>{sheetFor(phone)}</>);
    await userEvent.click(screen.getByText("Playback details"));
    expect(phone.toggleSheet).toHaveBeenCalledWith("details");
    expect(phone.setDetailsOpen).not.toHaveBeenCalled();
  });
});
