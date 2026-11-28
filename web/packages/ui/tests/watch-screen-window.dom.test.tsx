// @vitest-environment jsdom

import { idleTune, tuneView } from "@loomarr/core/tune";
import { type WatchRequest, watchKeyOf } from "@loomarr/core/watch-screen";
import { act, cleanup, fireEvent, renderUi, screen, within } from "@loomarr/test-support";
import { useEffect } from "react";
import { Text } from "react-native";
import { afterEach, describe, expect, it, vi } from "vitest";

import { type StripAiring, useWatchScreen, WatchScreen, type WatchScreenLayout } from "../index";

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

const channels = [1, 2, 3, 4].map((number) => ({
  id: `ch-${number}`,
  name: `Station ${number}`,
  number,
  onNow: `Programme ${number}`,
  playable: true,
}));
const lineup = { channels };

const minute = 60_000;
const NOW = 45 * minute;
const formatTime = (ms: number) => `T${ms / minute}`;
// Each channel's day: what is on now, and two later.
const airingsOf = (number: number): StripAiring[] =>
  [0, 60, 90].map((start) => ({
    description: `Synopsis ${number} at ${start}`,
    kind: "program",
    scheduleBlockId: `s${number}-${start}`,
    startMs: start * minute,
    stopMs: (start === 0 ? 60 : start + 30) * minute,
    title: `Show ${number} at ${start}`,
  }));

const requests = vi.fn<(request: WatchRequest) => void>();
const tunes = () =>
  requests.mock.calls.flatMap(([request]) => (request.type === "tune" ? [request.channelId] : []));
const back = vi.fn();

const Harness = ({ layout = "window" }: { layout?: WatchScreenLayout }) => {
  const model = useWatchScreen({ hintSeen: true, layout, lineup, onRequest: requests, watchedId: "ch-1" });
  const { send } = model;
  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      const target = event.target as HTMLElement | null;
      const key = watchKeyOf({
        defaultPrevented: event.defaultPrevented,
        inField: target?.tagName === "INPUT",
        key: event.key,
      });
      if (key !== undefined) send({ key, type: "key" });
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [send]);
  return (
    <WatchScreen
      back={{ label: "Home", onPress: back }}
      layout={layout}
      model={model}
      picture={<Text>picture</Text>}
      programme={(channel) => ({ kind: "on", minutesLeft: 15, title: `Show ${channel.number} at 0` })}
      schedule={{ airings: (channel) => airingsOf(channel.number), formatTime, nowMs: NOW }}
      tune={tuneView(idleTune, 0)}
    />
  );
};

type Measured = HTMLElement & { __reactLayoutHandler?: (event: unknown) => void };
// jsdom has no layout: the screen's size is handed to react-native-web's layout handler.
const measure = (width: number, height = 900) => {
  const root = screen.getByTestId("watch-screen") as Measured;
  act(() => root.__reactLayoutHandler?.({ nativeEvent: { layout: { height, width, x: 0, y: 0 } } }));
};

const row = (number: number) => screen.getByRole("option", { name: new RegExp(`^${number} `) });
const label = () => screen.getByTestId("watch-window-label").textContent;
const facts = () => screen.getByRole("group", { name: "Programme facts" }).textContent ?? "";
const key = (name: string, target: Element = document.activeElement ?? document.body) =>
  fireEvent.keyDown(target, { key: name });

describe("WatchScreen in a window", () => {
  it("draws the list beside the picture with the back control and the Guide in its header", () => {
    renderUi(<Harness />);
    expect(screen.getByRole("listbox", { name: "Channels" })).toBeTruthy();
    const head = within(screen.getByTestId("watch-window-head"));
    expect(head.getAllByRole("button").map((each) => each.getAttribute("aria-label"))).toEqual([
      "Home",
      "Guide",
    ]);
    fireEvent.click(head.getByRole("button", { name: "Home" }));
    expect(back).toHaveBeenCalledTimes(1);
    fireEvent.click(head.getByRole("button", { name: "Guide" }));
    expect(requests).toHaveBeenLastCalledWith({ type: "guide" });
    expect(label()).toBe("Today on Station 1");
    expect(facts()).toContain("Synopsis 1 at 0");
    expect(screen.queryByTestId("watch-scrim-foot")).toBeNull();
  });

  it("looks at a channel a pointer rests on, names both channels, and tunes nothing", () => {
    vi.useFakeTimers();
    renderUi(<Harness />);
    fireEvent.pointerEnter(row(3), { pointerType: "mouse" });
    act(() => vi.advanceTimersByTime(200));
    expect(label()).toBe("Looking at 3 Station 3 · still playing 1 Station 1 · click to tune");
    expect(facts()).toContain("Synopsis 3 at 0");
    expect(screen.getByRole("button", { name: /Show 3 at 60/ })).toBeTruthy();
    expect(tunes()).toEqual([]);
    fireEvent.pointerLeave(row(3), { pointerType: "mouse" });
    expect(label()).toBe("Today on Station 1");
    expect(facts()).toContain("Synopsis 1 at 0");
  });

  it("shows a later programme's facts while a pointer is on it in the strip", () => {
    renderUi(<Harness />);
    fireEvent.pointerEnter(screen.getByRole("button", { name: /Show 1 at 90/ }), { pointerType: "mouse" });
    expect(facts()).toContain("Synopsis 1 at 90");
    expect(facts()).toContain("Starts T90");
  });

  it("tunes on a click on a row, and ends looking", () => {
    vi.useFakeTimers();
    renderUi(<Harness />);
    fireEvent.pointerEnter(row(3), { pointerType: "mouse" });
    act(() => vi.advanceTimersByTime(200));
    fireEvent.click(row(3), { clientX: 40, clientY: 60 });
    expect(tunes()).toEqual(["ch-3"]);
    expect(label()).toBe("Today on Station 3");
    expect(screen.getByRole("listbox", { name: "Channels" })).toBeTruthy();
  });

  it("never changes channel on the wheel", () => {
    renderUi(<Harness />);
    fireEvent.wheel(screen.getByRole("listbox", { name: "Channels" }), { deltaY: 120 });
    fireEvent.wheel(screen.getByTestId("watch-picture"), { deltaY: 120 });
    expect(tunes()).toEqual([]);
  });

  it("flips on Up and Down from the bare picture", () => {
    renderUi(<Harness />);
    key("ArrowDown", document.body);
    expect(tunes()).toEqual(["ch-2"]);
  });

  it("walks the rows from a focused row without tuning, and Esc returns focus to the picture", () => {
    renderUi(<Harness />);
    act(() => row(2).focus());
    key("ArrowDown");
    expect(document.activeElement).toBe(row(3));
    expect(tunes()).toEqual([]);
    key("Escape");
    expect(document.activeElement).toBe(document.body);
    expect(screen.getByRole("listbox", { name: "Channels" })).toBeTruthy();
  });

  it("takes Left and Right from the bare picture into the film strip", () => {
    renderUi(<Harness />);
    key("ArrowRight", document.body);
    expect(document.activeElement?.getAttribute("aria-label")).toBe("T60 Show 1 at 60");
    expect(facts()).toContain("Synopsis 1 at 60");
  });

  it("takes focus off the header's controls on a pointer press anywhere in the window's chrome", () => {
    renderUi(<Harness />);
    const guide = within(screen.getByTestId("watch-window-head")).getByRole("button", { name: "Guide" });
    act(() => guide.focus());
    fireEvent.mouseDown(screen.getByTestId("watch-window-line"));
    expect(document.activeElement).toBe(document.body);
    act(() => guide.focus());
    fireEvent.mouseDown(screen.getByTestId("watch-window-head"));
    expect(document.activeElement).toBe(document.body);
  });

  it("pauses on a click on the picture", () => {
    renderUi(<Harness />);
    fireEvent.click(screen.getByTestId("watch-picture").firstElementChild as HTMLElement);
    expect(requests).toHaveBeenLastCalledWith({ type: "pause" });
  });

  it("keeps the channel and the group across a change to full screen and back", () => {
    const { rerender } = renderUi(<Harness />);
    fireEvent.click(screen.getByRole("tab", { name: "Favourites" }));
    rerender(<Harness layout="full" />);
    expect(screen.queryByRole("listbox", { name: "Channels" })).toBeNull();
    rerender(<Harness layout="window" />);
    expect(screen.getByRole("tab", { name: "Favourites", selected: true })).toBeTruthy();
    expect(label()).toBe("Today on Station 1");
  });
});

describe("WatchScreen in a narrow window", () => {
  it("stacks below 900 px: the picture, the lower third, then the whole list with no strip", () => {
    renderUi(<Harness />);
    measure(899);
    expect(screen.getByTestId("watch-window-narrow")).toBeTruthy();
    expect(screen.queryByTestId("watch-window")).toBeNull();
    expect(screen.queryByRole("group", { name: "Schedule" })).toBeNull();
    expect(screen.getAllByRole("option")).toHaveLength(4);
    expect(within(screen.getByTestId("watch-window-line")).getByTestId("lower-third-title").textContent).toBe(
      "Show 1 at 0",
    );
  });

  it("draws the columns from 900 px", () => {
    renderUi(<Harness />);
    measure(900);
    expect(screen.getByTestId("watch-window")).toBeTruthy();
    expect(screen.getByRole("group", { name: "Schedule" })).toBeTruthy();
  });
});
