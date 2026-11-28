// @vitest-environment jsdom

import { cleanup, renderUi, screen, userEvent } from "@loomarr/test-support";
import { afterEach, describe, expect, it, vi } from "vitest";

import { FirstRunHint, LowerThird, type LowerThirdProps, type LowerThirdState } from "../index";

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

const group = () => screen.getByRole("group", { name: "Channel 7, Coast 90s" });

describe("LowerThird", () => {
  it("reads one summary and hides the drawn numeral and facts from a screen reader", () => {
    third({ decade: 1990 });
    expect(group().textContent).toContain(
      "Channel 7, Coast 90s. The Diner Years. Rated TV-PG. 12 minutes left. Next at 10:30, Night Desk.",
    );
    for (const id of ["lower-third-numeral", "lower-third-title"]) {
      expect(screen.getByTestId(id).closest('[aria-hidden="true"]')).not.toBeNull();
    }
    expect(group().textContent).toContain("Coast 90s · TV-PG · 12 min left");
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
    expect(group().textContent).toContain("Coast 90s · 12 min left");
    await userEvent.click(screen.getByRole("button", { name: "Pause" }));
    await userEvent.click(screen.getByRole("button", { name: "More" }));
    expect(onTogglePause).toHaveBeenCalledTimes(1);
    expect(onMore).toHaveBeenCalledTimes(1);
  });

  it("names the channel while tuning in, and says nothing is scheduled with no Next", () => {
    const { unmount } = third({ state: { kind: "tuning" } });
    expect(screen.getByTestId("lower-third-title").textContent).toBe("Coast 90s");
    expect(group().textContent).toContain("Tuning in… Starting the channel");
    unmount();
    third({ state: { kind: "nothing" } });
    expect(group().textContent).toContain("Nothing scheduled");
    expect(group().textContent).not.toContain("Next");
  });

  it("names the programme coming back from a break, and says Paused", () => {
    const { unmount } = third({ state: { backInMinutes: 3, kind: "break", title: "The Diner Years" } });
    expect(group().textContent).toContain("Back in 3 min");
    unmount();
    third({ state: { ...on, paused: true } as LowerThirdState });
    expect(group().textContent).toContain("TV-PG · Paused");
    expect(screen.queryByRole("button")).toBeNull();
  });

  it("offers Go live behind live, except on a television", async () => {
    const onGoLive = vi.fn();
    const behind = { ...on, behindSeconds: 40 } as LowerThirdState;
    const { unmount } = third({ onGoLive, state: behind });
    expect(group().textContent).toContain("40 s behind live");
    await userEvent.click(screen.getByRole("button", { name: "Go live" }));
    expect(onGoLive).toHaveBeenCalledTimes(1);
    unmount();
    third({ device: "tv", state: { ...on, behindSeconds: 150 } as LowerThirdState });
    expect(group().textContent).toContain("2 min behind");
    expect(screen.queryByRole("button", { name: "Go live" })).toBeNull();
  });

  it("replaces the numeral with typed digits and says what Enter will do", () => {
    const { unmount } = third({
      state: { digits: "12", kind: "typing", target: { name: "Movie Matinee", onNow: "A film" } },
    });
    expect(screen.getByTestId("lower-third-numeral").textContent).toBe("12");
    expect(screen.getByTestId("lower-third-typing-rule")).toBeTruthy();
    expect(group().textContent).toContain("A film · press Enter, or wait");
    unmount();
    third({ state: { digits: "99", kind: "typing" } });
    expect(screen.getByTestId("lower-third-title").textContent).toBe("No such channel");
    expect(group().textContent).toContain("There is no channel 99");
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
