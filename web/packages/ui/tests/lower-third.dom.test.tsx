// @vitest-environment jsdom

import { cleanup, renderUi, screen, userEvent } from "@loomarr/test-support";
import { createRef } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import {
  FirstRunHint,
  LowerThird,
  type LowerThirdProps,
  type LowerThirdState,
  QuietRow,
  type QuietRowHandle,
} from "../index";

afterEach(cleanup);

const on: LowerThirdState = {
  kind: "on",
  minutesLeft: 12,
  next: { startsAt: "10:30", title: "Night Desk" },
  rating: "TV-PG",
  title: "The Diner Years",
};

const third = (props: Partial<LowerThirdProps> = {}) =>
  renderUi(<LowerThird channelName="Coast 90s" channelNumber={7} device="full" state={on} {...props} />);

/** Every word in the part: the spoken summary first, then what is drawn. */
const words = () => document.body.textContent;

const colourOf = (testId: string) => getComputedStyle(screen.getByTestId(testId)).color;

describe("LowerThird", () => {
  it("reads one summary and hides the drawn numeral and facts from a screen reader", () => {
    third({ decade: 1990 });
    const summary =
      "Channel 7, Coast 90s. The Diner Years. Rated TV-PG. 12 minutes left. Next at 10:30, Night Desk.";
    expect(screen.getByText(summary).closest('[aria-hidden="true"]')).toBeNull();
    for (const id of ["lower-third-numeral", "lower-third-title"]) {
      expect(screen.getByTestId(id).closest('[aria-hidden="true"]')).not.toBeNull();
    }
    expect(words()).toContain("Coast 90s · TV-PG · 12 min left");
    // The summary is the one name: nothing around it is labelled, so it is not read twice.
    expect(screen.queryByRole("group")).toBeNull();
    expect(document.querySelector("[aria-label]")).toBeNull();
  });

  it("says the typed channel, not the watched one, while a number is typed", () => {
    third({ state: { digits: "12", kind: "typing", target: { name: "Movie Matinee" } } });
    expect(screen.getByText("Channel 12, Movie Matinee. Press Enter, or wait.")).toBeTruthy();
    expect(words()).not.toContain("Coast 90s");
  });

  it.each([
    [0, "1 min left", "1 minute left"],
    [1, "1 min left", "1 minute left"],
    [2, "2 min left", "2 minutes left"],
  ])("never says 0 minutes: %i reads as %s", (minutesLeft, drawn, spoken) => {
    third({ state: { ...on, minutesLeft } as LowerThirdState });
    expect(words()).toContain(drawn);
    expect(words()).toContain(spoken);
    cleanup();
    third({ state: { backInMinutes: minutesLeft, kind: "break", title: "The Diner Years" } });
    expect(words()).toContain(`Back in ${Math.max(1, minutesLeft)} min`);
    expect(words()).toContain(`is back in ${spoken.replace(" left", "")}`);
  });

  it.each([
    [59, "59 s behind live", "59 seconds behind live"],
    [59.6, "59 s behind live", "59 seconds behind live"],
    [60, "1 min behind", "1 minute behind live"],
    [61, "1 min behind", "1 minute behind live"],
  ])("counts only what has passed behind live: %d s", (behindSeconds, drawn, spoken) => {
    third({ state: { ...on, behindSeconds } as LowerThirdState });
    expect(words()).toContain(drawn);
    expect(words()).toContain(spoken);
  });

  it("tints only the watched channel's numeral, neutral with no decade, never typed digits", () => {
    const seen = (props: Partial<LowerThirdProps>) => {
      third(props);
      const drawn = { numeral: colourOf("lower-third-numeral"), title: colourOf("lower-third-title") };
      cleanup();
      return drawn;
    };
    const nineties = seen({ decade: 1990 });
    const none = seen({});
    const typed = seen({ decade: 1990, state: { digits: "12", kind: "typing" } });
    expect(nineties.numeral).toContain("decadeTint1990");
    expect(none.numeral).toContain("decadeTintNeutral");
    expect(typed.numeral).not.toContain("decadeTint");
    expect(nineties.title).not.toContain("decadeTint");
  });

  it("keeps the dark set over the picture in the light theme, and follows the theme beside it", () => {
    const light = (device: LowerThirdProps["device"]) => {
      renderUi(
        <LowerThird channelName="Coast 90s" channelNumber={7} decade={1990} device={device} state={on} />,
        { theme: "light" },
      );
      // The nearest theme scope around the numeral decides which set its tint takes.
      const scope = screen.getByTestId("lower-third-numeral").closest(".t_dark, .t_light");
      const theme = scope?.classList.contains("t_dark") ? "dark" : "light";
      cleanup();
      return theme;
    };
    expect(light("full")).toBe("dark");
    expect(light("window")).toBe("light");
  });

  it("sends focus on when Go live leaves from under it", async () => {
    const behind = { ...on, behindSeconds: 40 } as LowerThirdState;
    const screenOf = (device: LowerThirdProps["device"], state: LowerThirdState) => {
      const row = createRef<QuietRowHandle>();
      return (
        <>
          <LowerThird
            channelName="Coast 90s"
            channelNumber={7}
            device={device}
            onGoLiveDropped={() => row.current?.focusHead()}
            state={state}
          />
          <QuietRow device={device} onPress={() => undefined} ref={row} />
        </>
      );
    };
    for (const [device, head] of [
      ["full", "Channels"],
      ["portrait", "Pause"],
    ] as const) {
      const view = renderUi(screenOf(device, behind));
      screen.getByRole("button", { name: "Go live" }).focus();
      view.rerender(screenOf(device, on));
      expect(document.activeElement?.getAttribute("aria-label"), device).toBe(head);
      cleanup();
    }
  });

  it.each([
    ["full", "64px", "34px"],
    ["window", "56px", "28px"],
    ["tv", "52px", "30px"],
    ["portrait", "44px", "22px"],
    ["landscape", "40px", "22px"],
  ] as const)("sizes the numeral and title for %s", (device, numeral, title) => {
    third({ device });
    expect(getComputedStyle(screen.getByTestId("lower-third-numeral")).fontSize).toBe(numeral);
    expect(getComputedStyle(screen.getByTestId("lower-third-title")).fontSize).toBe(title);
  });

  it("leaves the rating off an upright phone's short line and puts Pause and More beside it", async () => {
    const onTogglePause = vi.fn();
    const onMore = vi.fn();
    third({ device: "portrait", onMore, onTogglePause });
    expect(words()).toContain("Coast 90s · 12 min left");
    await userEvent.click(screen.getByRole("button", { name: "Pause" }));
    await userEvent.click(screen.getByRole("button", { name: "More" }));
    expect(onTogglePause).toHaveBeenCalledTimes(1);
    expect(onMore).toHaveBeenCalledTimes(1);
  });

  it("names the channel while tuning in, and says nothing is scheduled with no Next", () => {
    const { unmount } = third({ state: { kind: "tuning" } });
    expect(screen.getByTestId("lower-third-title").textContent).toBe("Coast 90s");
    expect(words()).toContain("Tuning in… Starting the channel");
    unmount();
    third({ state: { kind: "nothing" } });
    expect(words()).toContain("Nothing scheduled");
    expect(words()).not.toContain("Next");
  });

  it("names the programme coming back from a break, and says Paused", () => {
    const { unmount } = third({ state: { backInMinutes: 3, kind: "break", title: "The Diner Years" } });
    expect(words()).toContain("Back in 3 min");
    unmount();
    third({ state: { ...on, paused: true } as LowerThirdState });
    expect(words()).toContain("TV-PG · Paused");
    expect(screen.queryByRole("button")).toBeNull();
  });

  it("offers Go live behind live, except on a television", async () => {
    const onGoLive = vi.fn();
    const behind = { ...on, behindSeconds: 40 } as LowerThirdState;
    const { unmount } = third({ onGoLive, state: behind });
    expect(words()).toContain("40 s behind live");
    await userEvent.click(screen.getByRole("button", { name: "Go live" }));
    expect(onGoLive).toHaveBeenCalledTimes(1);
    unmount();
    third({ device: "tv", state: { ...on, behindSeconds: 150 } as LowerThirdState });
    expect(words()).toContain("2 min behind");
    expect(screen.queryByRole("button", { name: "Go live" })).toBeNull();
  });

  it("replaces the numeral with typed digits and says what Enter will do", () => {
    const { unmount } = third({
      state: { digits: "12", kind: "typing", target: { name: "Movie Matinee", onNow: "A film" } },
    });
    expect(screen.getByTestId("lower-third-numeral").textContent).toBe("12");
    expect(screen.getByTestId("lower-third-typing-rule")).toBeTruthy();
    expect(words()).toContain("A film · press Enter, or wait");
    unmount();
    third({ state: { digits: "99", kind: "typing" } });
    expect(screen.getByTestId("lower-third-title").textContent).toBe("No such channel");
    expect(words()).toContain("There is no channel 99");
  });
});

describe("LowerThird in a screen's arrangement", () => {
  const behind = { ...on, behindSeconds: 40 } as LowerThirdState;

  it("draws no ink of its own when the screen draws the scrim, and stays dark", () => {
    third();
    expect(screen.getByTestId("picture-ink-side")).toBeTruthy();
    cleanup();
    renderUi(
      <LowerThird channelName="Coast 90s" channelNumber={7} device="full" ownInk={false} state={on} />,
      { theme: "light" },
    );
    expect(screen.queryByTestId("picture-ink-side")).toBeNull();
    const scope = screen.getByTestId("lower-third-numeral").closest(".t_dark, .t_light");
    expect(scope?.classList.contains("t_dark")).toBe(true);
  });

  it("draws the words alone without its controls", () => {
    third({ controls: false, state: behind });
    expect(screen.queryByRole("button", { name: "Go live" })).toBeNull();
    expect(words()).toContain("40 s behind live");
    cleanup();
    third({ controls: false, device: "portrait" });
    expect(screen.queryAllByRole("button")).toHaveLength(0);
  });

  it("says when Go live takes and lets go of the keyboard's focus, and never a click's", async () => {
    const onGoLiveFocus = vi.fn();
    third({ onGoLiveFocus, state: behind });
    const goLive = screen.getByRole("button", { name: "Go live" });
    // A pointer takes no focus, so a key after the click is not Go live's.
    await userEvent.click(goLive);
    expect(document.activeElement).toBe(document.body);
    await userEvent.keyboard("{ArrowDown}");
    expect(onGoLiveFocus).not.toHaveBeenCalled();
    await userEvent.tab();
    expect(onGoLiveFocus.mock.calls).toEqual([[true]]);
    await userEvent.tab();
    expect(onGoLiveFocus.mock.calls).toEqual([[true], [false]]);
  });

  it("names Go live's key, End, in its tooltip on a desktop", async () => {
    third({ state: behind });
    await userEvent.hover(screen.getByRole("button", { name: "Go live" }));
    expect((await screen.findByRole("tooltip")).textContent).toBe("Go live (End)");
  });
});

describe("FirstRunHint", () => {
  it("words each device, and a phone on its side has none", () => {
    const { unmount } = renderUi(<FirstRunHint device="portrait" />);
    expect(screen.getByRole("note").textContent).toBe("Tap a channel to change it");
    unmount();
    renderUi(<FirstRunHint device="landscape" />);
    expect(screen.queryByRole("note")).toBeNull();
  });
});
