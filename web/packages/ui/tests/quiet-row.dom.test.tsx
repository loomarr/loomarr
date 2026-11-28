// @vitest-environment jsdom

import { cleanup, renderUi, screen, userEvent } from "@loomarr/test-support";
import { afterEach, describe, expect, it, vi } from "vitest";

import { QuietRow, type QuietRowProps } from "../index";

afterEach(() => {
  cleanup();
  Element.prototype.matches = nativeMatches;
});

// jsdom answers `:focus-visible` with false whatever gave the focus; a browser answers true for the
// keyboard, and the stories check the ring there.
const nativeMatches = Element.prototype.matches;
const keyboardFocus = () => {
  Element.prototype.matches = new Proxy(nativeMatches, {
    apply: (original, element, args: [string]) =>
      args[0] === ":focus-visible" || Reflect.apply(original, element, args),
  });
};

const row = (props: Partial<QuietRowProps> = {}) =>
  renderUi(<QuietRow device="full" onPress={() => undefined} {...props} />);

const names = () => screen.getAllByRole("button").map((b) => b.getAttribute("aria-label"));

describe("QuietRow", () => {
  it.each([
    [
      "full",
      [
        "Channels",
        "Guide",
        "Pause (Space)",
        "Subtitles (C)",
        "Audio (A)",
        "Mute (M)",
        "Leave full screen (F)",
        "More",
      ],
    ],
    ["window", ["Pause (Space)", "Subtitles (C)", "Audio (A)", "Mute (M)", "Full screen (F)", "More"]],
    ["tv", ["Channels", "Guide", "Pause", "Subtitles", "Audio", "More"]],
    ["landscape", ["Channels", "Guide", "Pause", "Subtitles", "Audio", "AirPlay", "More"]],
    ["portrait", ["Subtitles", "Audio", "AirPlay"]],
  ] as const)("holds the %s controls in order", (device, expected) => {
    row({ device });
    expect(names()).toEqual(expected);
  });

  it("writes the words of Channels and Guide, and names a group other than All", () => {
    const { unmount } = row({ group: "All" });
    expect(screen.getByRole("button", { name: "Channels" }).textContent).toBe("Channels");
    expect(screen.getByRole("button", { name: "Guide" }).textContent).toBe("Guide");
    unmount();
    row({ group: "Favourites", listOpen: true });
    const channels = screen.getByRole("button", { name: "Channels: Favourites" });
    expect(channels.getAttribute("aria-expanded")).toBe("true");
  });

  it("leads a television with Go live only while behind live and playing", () => {
    const { unmount } = row({ behindLive: true, device: "tv" });
    expect(names()[0]).toBe("Go live");
    unmount();
    const paused = row({ behindLive: true, device: "tv", paused: true });
    expect(names()).not.toContain("Go live");
    paused.unmount();
    row({ behindLive: true, device: "full" });
    expect(names()).not.toContain("Go live");
  });

  it("is one Tab stop with Left, Right, Home and End inside, and draws one amber ring", async () => {
    const onPress = vi.fn();
    keyboardFocus();
    row({ onPress });
    expect(screen.getByRole("toolbar", { name: "Player controls" })).toBeTruthy();
    const focused = () => document.activeElement?.getAttribute("aria-label");

    await userEvent.tab();
    expect(focused()).toBe("Channels");
    await userEvent.keyboard("{ArrowLeft}{ArrowRight}{ArrowRight}");
    expect(focused()).toBe("Pause (Space)");
    expect(screen.getAllByTestId("ring-actionFocus")).toHaveLength(1);
    await userEvent.keyboard("{Enter}");
    expect(onPress).toHaveBeenLastCalledWith("pause");
    await userEvent.keyboard("{End}");
    expect(focused()).toBe("More");
    await userEvent.keyboard("{Home}");
    expect(focused()).toBe("Channels");
    await userEvent.tab();
    expect(document.activeElement).toBe(document.body);
  });

  it("moves focus to Channels when Go live leaves a television's row from under it", async () => {
    const props = { behindLive: true, device: "tv", onPress: () => undefined } as const;
    const view = renderUi(<QuietRow {...props} />);
    await userEvent.tab();
    expect(document.activeElement?.getAttribute("aria-label")).toBe("Go live");
    view.rerender(<QuietRow {...props} behindLive={false} />);
    expect(document.activeElement?.getAttribute("aria-label")).toBe("Channels");
  });

  it("leaves focus alone when Go live goes while another control holds it", async () => {
    const props = { behindLive: true, device: "tv", onPress: () => undefined } as const;
    const view = renderUi(<QuietRow {...props} />);
    await userEvent.tab();
    await userEvent.keyboard("{End}");
    view.rerender(<QuietRow {...props} behindLive={false} />);
    expect(document.activeElement?.getAttribute("aria-label")).toBe("More");
  });

  it("draws a plain ring under a pointer, never the amber one", async () => {
    row();
    await userEvent.hover(screen.getByRole("button", { name: "Guide" }));
    expect(screen.getAllByTestId("ring-contentPrimary")).toHaveLength(1);
    await userEvent.click(screen.getByRole("button", { name: "Guide" }));
    expect(screen.queryByTestId("ring-actionFocus")).toBeNull();
  });

  it("draws no ink of its own when the screen draws the scrim", () => {
    row();
    expect(screen.getByTestId("picture-ink-foot")).toBeTruthy();
    cleanup();
    row({ ownInk: false });
    expect(screen.queryByTestId("picture-ink-foot")).toBeNull();
    expect(names()).toHaveLength(8);
  });

  it("says which control holds focus, and when focus leaves the row", async () => {
    const onFocusKey = vi.fn();
    row({ onFocusKey });
    await userEvent.tab();
    await userEvent.keyboard("{ArrowRight}");
    await userEvent.tab();
    expect(onFocusKey.mock.calls).toEqual([["channels"], [undefined], ["guide"], [undefined]]);
  });

  it("says nothing of focus a click gave, and keeps the control said when moving left", async () => {
    const onFocusKey = vi.fn();
    row({ onFocusKey });
    await userEvent.click(screen.getByRole("button", { name: "Mute (M)" }));
    expect(onFocusKey).not.toHaveBeenCalled();
    keyboardFocus();
    await userEvent.keyboard("{ArrowLeft}");
    expect(onFocusKey).toHaveBeenLastCalledWith("audio");
    await userEvent.keyboard("{ArrowLeft}");
    expect(onFocusKey).toHaveBeenLastCalledWith("subtitles");
  });

  it("names an icon in a tooltip with its key on a desktop", async () => {
    row();
    await userEvent.hover(screen.getByRole("button", { name: "Subtitles (C)" }));
    expect((await screen.findByRole("tooltip")).textContent).toBe("Subtitles (C)");
  });

  it("names a television's focused icon with no key", async () => {
    keyboardFocus();
    row({ device: "tv" });
    await userEvent.tab();
    await userEvent.keyboard("{ArrowRight}{ArrowRight}");
    const pause = screen.getByRole("button", { name: "Pause" });
    expect(pause.textContent).toBe("Pause");
  });
});
