// @vitest-environment jsdom

import { idleTune, tuneView } from "@loomarr/core/tune";
import { type WatchRequest, watchKeyOf } from "@loomarr/core/watch-screen";
import { act, cleanup, fireEvent, renderUi, screen, userEvent, within } from "@loomarr/test-support";
import { useEffect } from "react";
import { Platform, Text } from "react-native";
import { afterEach, describe, expect, it, vi } from "vitest";

import { type PlayerWatermark, useWatchScreen, WatchScreen } from "../index";

// A desktop's window, for the logo's clearance.
vi.mock("react-native", async (importOriginal) => {
  const actual = await importOriginal<typeof import("react-native")>();
  return { ...actual, useWindowDimensions: () => ({ fontScale: 1, height: 720, scale: 1, width: 1280 }) };
});

const nativeMatches = Element.prototype.matches;
afterEach(() => {
  cleanup();
  vi.useRealTimers();
  Element.prototype.matches = nativeMatches;
  Reflect.deleteProperty(document, "elementFromPoint");
});

// jsdom answers `:focus-visible` with false whatever gave the focus; a browser answers true for the
// keyboard, and the stories check the ring there.
const keyboardFocus = () => {
  Element.prototype.matches = new Proxy(nativeMatches, {
    apply: (original, element, args: [string]) =>
      args[0] === ":focus-visible" || Reflect.apply(original, element, args),
  });
};

const channels = [1, 2, 3, 4].map((number) => ({
  id: `ch-${number}`,
  name: `Station ${number}`,
  number,
  onNow: `Programme ${number}`,
  playable: true,
}));
const lineup = { channels };

const requests = vi.fn<(request: WatchRequest) => void>();
const tunes = () =>
  requests.mock.calls.flatMap(([request]) => (request.type === "tune" ? [request.channelId] : []));

const focusOf: { current: string | undefined } = { current: undefined };

// The caller's part: the model, the document's keys, and slots for a menu.
const Harness = ({
  back,
  cover = false,
  fullScreen,
  hintSeen = true,
  watermark,
}: {
  back?: { label: string; onPress: () => void };
  cover?: boolean;
  fullScreen?: boolean;
  hintSeen?: boolean;
  watermark?: PlayerWatermark;
}) => {
  const model = useWatchScreen({ hintSeen, layout: "full", lineup, onRequest: requests, watchedId: "ch-1" });
  const { send } = model;
  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      const target = event.target as HTMLElement | null;
      const key = watchKeyOf({
        altKey: event.altKey,
        composing: event.isComposing,
        ctrlKey: event.ctrlKey,
        defaultPrevented: event.defaultPrevented,
        inField: target?.tagName === "INPUT" || target?.tagName === "TEXTAREA",
        key: event.key,
        metaKey: event.metaKey,
      });
      if (key !== undefined) send({ key, type: "key" });
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [send]);
  // What the model holds focused, for the tests to read.
  focusOf.current = model.state.focus;
  return (
    <WatchScreen
      back={back}
      fullScreen={fullScreen}
      layout="full"
      watermark={watermark}
      model={model}
      cover={
        cover ? (
          <Text accessibilityRole="button" onPress={() => undefined}>
            Off air
          </Text>
        ) : undefined
      }
      picture={<Text>picture</Text>}
      programme={(channel) => ({
        kind: "on",
        minutesLeft: 10 + channel.number,
        title: `Show ${channel.number}`,
      })}
      renderMenu={({ name, page, openPage }) => (
        <Text accessibilityRole="button" onPress={() => openPage("inner")}>
          {`menu ${name} ${page ?? "first"}`}
        </Text>
      )}
      tune={tuneView(idleTune, 0)}
    />
  );
};

const key = (name: string, target: Element = document.activeElement ?? document.body) =>
  fireEvent.keyDown(target, { key: name });
const listbox = () => screen.queryByRole("listbox", { name: "Channels" });
const channelsControl = () => screen.getByRole("button", { name: "Channels" });

// A browser's answer for focus that follows a click, programmatic focus included: not the keyboard's.
const pointerFocus = () => {
  Element.prototype.matches = new Proxy(nativeMatches, {
    apply: (original, element, args: [string]) =>
      args[0] === ":focus-visible" ? false : Reflect.apply(original, element, args),
  });
};

// A click as a browser gives it: the pointer goes down, the control takes focus unless the
// mousedown's default was prevented, and the press lands.
const click = (element: HTMLElement) => {
  fireEvent.pointerDown(element, { pointerType: "mouse" });
  if (fireEvent.mouseDown(element)) act(() => element.focus());
  fireEvent.click(element);
};

const restedAfter = (ms: number) => {
  act(() => {
    vi.advanceTimersByTime(ms);
  });
  return screen.queryByRole("note") === null;
};

describe("WatchScreen and focus a pointer meets", () => {
  it("takes the keyboard's ring off Channels when the pointer presses Mute", () => {
    vi.useFakeTimers();
    keyboardFocus();
    renderUi(<Harness hintSeen={false} />);
    act(() => channelsControl().focus());
    expect(screen.getAllByTestId("ring-actionFocus")).toHaveLength(1);
    click(screen.getByRole("button", { name: "Mute (M)" }));
    expect(screen.queryByTestId("ring-actionFocus")).toBeNull();
    expect(document.activeElement).toBe(document.body);
    fireEvent.pointerLeave(screen.getByTestId("watch-quiet-row"));
    expect(restedAfter(5000)).toBe(true);
    key(" ", document.body);
    expect(requests.mock.calls.map(([request]) => request.type)).toEqual(["mute", "pause"]);
    requests.mockClear();
  });

  it("takes the keyboard's ring off Go live when the pointer presses the quiet row", () => {
    vi.useFakeTimers({ toFake: ["Date"] });
    keyboardFocus();
    renderUi(<Harness />);
    key(" ", document.body);
    vi.setSystemTime(Date.now() + 5000);
    key(" ", document.body);
    const goLive = screen.getByRole("button", { name: "Go live" });
    act(() => goLive.focus());
    expect(screen.getAllByTestId("ring-actionFocus")).toHaveLength(1);
    expect(focusOf.current).toBe("goLive");
    click(screen.getByRole("button", { name: "Mute (M)" }));
    expect(document.activeElement).toBe(document.body);
    expect(screen.queryByTestId("ring-actionFocus")).toBeNull();
    expect(focusOf.current).toBeUndefined();
    requests.mockClear();
  });

  // Behind live, with the keyboard on one control; then the pointer presses Go live.
  const clickGoLiveFrom = (focusOn: () => HTMLElement) => {
    vi.useFakeTimers();
    keyboardFocus();
    renderUi(<Harness hintSeen={false} />);
    key(" ", document.body);
    act(() => {
      vi.advanceTimersByTime(5000);
    });
    key(" ", document.body);
    act(() => focusOn().focus());
    expect(screen.getAllByTestId("ring-actionFocus")).toHaveLength(1);
    requests.mockClear();
    click(screen.getByRole("button", { name: "Go live" }));
    expect(requests).toHaveBeenLastCalledWith({ type: "goLive" });
    expect(screen.queryByRole("button", { name: "Go live" })).toBeNull();
    // Nothing hands focus on after a pointer press: it stays on the page.
    expect(document.activeElement).toBe(document.body);
    expect(screen.queryByTestId("ring-actionFocus")).toBeNull();
    expect(focusOf.current).toBeUndefined();
    fireEvent.pointerLeave(screen.getByTestId("watch-lower-third"));
    expect(restedAfter(5000)).toBe(true);
    key(" ", document.body);
    expect(requests).toHaveBeenLastCalledWith({ type: "pause" });
    requests.mockClear();
  };

  it("leaves focus on the page after the keyboard's Go live is clicked", () => {
    clickGoLiveFrom(() => screen.getByRole("button", { name: "Go live" }));
  });

  it("leaves focus on the page after Go live is clicked while the keyboard is on Mute", () => {
    clickGoLiveFrom(() => screen.getByRole("button", { name: "Mute (M)" }));
  });

  // The pointer moves onto the lower third and stays: the page's answer for what is at its place is
  // the lower third, whenever the lower third is drawn.
  const pointerOnLowerThird = () => {
    fireEvent.pointerMove(screen.getByTestId("watch-screen"), { clientX: 120, clientY: 600 });
    Object.defineProperty(document, "elementFromPoint", {
      configurable: true,
      value: (x: number, y: number) =>
        x === 120 && y === 600
          ? (screen.queryByTestId("watch-lower-third")?.querySelector("span") ?? null)
          : null,
    });
  };

  it("ends the hold the page's answer made once the pointer moves off the lower third, with no pointer-leave", () => {
    vi.useFakeTimers();
    pointerFocus();
    renderUi(<Harness hintSeen={false} />);
    pointerOnLowerThird();
    key("/", document.body);
    key("Escape", document.body);
    key("Escape", document.body);
    expect(restedAfter(60_000)).toBe(false);
    // The browser sends no pointer-leave for an element it never saw the pointer enter.
    fireEvent.pointerMove(screen.getByTestId("watch-screen"), { clientX: 5, clientY: 5 });
    expect(restedAfter(6000)).toBe(true);
  });

  it("holds nothing when the lower third comes back and no pointer has moved", () => {
    vi.useFakeTimers();
    Object.defineProperty(document, "elementFromPoint", {
      configurable: true,
      value: () => screen.queryByTestId("watch-lower-third")?.querySelector("span") ?? null,
    });
    renderUi(<Harness hintSeen={false} />);
    key("/", document.body);
    key("Escape", document.body);
    key("Escape", document.body);
    expect(restedAfter(6000)).toBe(true);
  });

  it("holds awake when the lower third comes back under a pointer that is already on it", () => {
    vi.useFakeTimers();
    pointerFocus();
    renderUi(<Harness hintSeen={false} />);
    pointerOnLowerThird();
    click(channelsControl());
    click(channelsControl());
    expect(listbox()).toBeNull();
    fireEvent.pointerLeave(screen.getByTestId("watch-quiet-row"));
    expect(restedAfter(60_000)).toBe(false);
    // The pointer goes, and the screen rests five seconds later.
    fireEvent.pointerLeave(screen.getByTestId("watch-lower-third"));
    expect(restedAfter(5000)).toBe(true);
  });

  it("holds awake under a still pointer when find opened the list and Esc closed it", () => {
    vi.useFakeTimers();
    pointerFocus();
    renderUi(<Harness hintSeen={false} />);
    pointerOnLowerThird();
    key("/", document.body);
    expect(listbox()).not.toBeNull();
    key("Escape", document.body);
    key("Escape", document.body);
    expect(listbox()).toBeNull();
    expect(restedAfter(60_000)).toBe(false);
    fireEvent.pointerLeave(screen.getByTestId("watch-lower-third"));
    expect(restedAfter(5000)).toBe(true);
  });

  it("hands focus back to Channels away from the web, where every press is a remote's", () => {
    const os = Platform.OS;
    (Platform as { OS: string }).OS = "android";
    try {
      renderUi(<Harness />);
      fireEvent.click(channelsControl());
      expect(listbox()).not.toBeNull();
      // OK on the watched channel closes the list.
      fireEvent.click(screen.getByRole("option", { selected: true }));
      expect(listbox()).toBeNull();
      expect(document.activeElement).toBe(channelsControl());
    } finally {
      (Platform as { OS: string }).OS = os;
    }
  });
});

const blur = () => act(() => (document.activeElement as HTMLElement | null)?.blur());
const mute = () => screen.getByRole("button", { name: "Mute (M)" });

describe("WatchScreen and the one note of the last input", () => {
  it("hands a list key pressed on the page to the list's cursor row", () => {
    renderUi(<Harness />);
    click(channelsControl());
    blur();
    key("ArrowDown", document.body);
    expect(document.activeElement?.getAttribute("role")).toBe("option");
    expect(tunes()).toEqual([]);
  });

  it("hands nothing on when a pointer closes a menu", () => {
    keyboardFocus();
    renderUi(<Harness />);
    key("a", document.body);
    click(screen.getByTestId("watch-picture").firstElementChild as HTMLElement);
    expect(screen.queryByText(/^menu audio/)).toBeNull();
    expect(document.activeElement).toBe(document.body);
  });

  it("hands nothing on, and draws no ring on Channels, when a row is clicked in a list a key opened", async () => {
    keyboardFocus();
    renderUi(<Harness />);
    act(() => channelsControl().focus());
    await userEvent.keyboard("{Enter}");
    expect(listbox()).not.toBeNull();
    await userEvent.click(screen.getByRole("option", { name: /^3 / }));
    expect(tunes()).toEqual(["ch-3"]);
    expect(document.activeElement).not.toBe(channelsControl());
    expect(within(channelsControl()).queryByTestId("ring-actionFocus")).toBeNull();
    requests.mockClear();
  });

  it("lands a key's hand-off with the ring, and stays awake while it holds focus", () => {
    vi.useFakeTimers();
    keyboardFocus();
    renderUi(<Harness hintSeen={false} />);
    key(" ", document.body);
    act(() => {
      vi.advanceTimersByTime(5000);
    });
    key(" ", document.body);
    const goLive = screen.getByRole("button", { name: "Go live" });
    act(() => goLive.focus());
    // End on Go live: it goes, and focus goes on to Channels.
    key("End", goLive);
    expect(document.activeElement).toBe(channelsControl());
    expect(within(channelsControl()).getByTestId("ring-actionFocus")).toBeTruthy();
    expect(focusOf.current).toBe("channels");
    expect(restedAfter(60_000)).toBe(false);
    // Esc closing the list Channels opened lands there too.
    click(channelsControl());
    key("Escape");
    expect(listbox()).toBeNull();
    expect(document.activeElement).toBe(channelsControl());
    expect(within(channelsControl()).getByTestId("ring-actionFocus")).toBeTruthy();
    expect(restedAfter(60_000)).toBe(false);
    requests.mockClear();
  });

  it("never blurs the find field, a menu item or the cover's control on a press on the foot", () => {
    renderUi(<Harness cover />);
    key("/", document.body);
    const find = screen.getByRole("searchbox");
    expect(document.activeElement).toBe(find);
    fireEvent.mouseDown(mute());
    expect(document.activeElement).toBe(find);
    key("Escape", document.body);
    key("Escape", document.body);
    key("a", document.body);
    const item = screen.getByText("menu audio first");
    act(() => item.focus());
    fireEvent.mouseDown(mute());
    expect(document.activeElement).toBe(item);
    const offAir = screen.getByText("Off air");
    act(() => offAir.focus());
    fireEvent.mouseDown(mute());
    expect(document.activeElement).toBe(offAir);
  });

  it("drops a foot control's keyboard focus on a press on the foot's empty space, and does nothing else", () => {
    keyboardFocus();
    renderUi(<Harness />);
    act(() => mute().focus());
    expect(screen.getAllByTestId("ring-actionFocus")).toHaveLength(1);
    fireEvent.pointerDown(screen.getByTestId("watch-quiet-row"));
    fireEvent.mouseDown(screen.getByTestId("watch-quiet-row").firstElementChild as Element);
    expect(document.activeElement).toBe(document.body);
    expect(screen.queryByTestId("ring-actionFocus")).toBeNull();
    expect(requests).not.toHaveBeenCalled();
  });
});

describe("WatchScreen at rest", () => {
  it("rests after a click on Mute, the pointer moving onto the picture, and a key", () => {
    vi.useFakeTimers();
    // Even were the browser to call any focus the keyboard's, a click gives none here.
    keyboardFocus();
    renderUi(<Harness hintSeen={false} />);
    click(screen.getByRole("button", { name: "Mute (M)" }));
    expect(document.activeElement).toBe(document.body);
    expect(requests).toHaveBeenLastCalledWith({ type: "mute" });
    fireEvent.pointerLeave(screen.getByTestId("watch-quiet-row"));
    fireEvent.pointerMove(screen.getByTestId("watch-picture"));
    key("ArrowDown");
    expect(restedAfter(6000)).toBe(true);
    expect(tunes()).toEqual(["ch-2"]);
    expect(screen.queryByTestId("ring-actionFocus")).toBeNull();
    requests.mockClear();
  });

  it("rests after Channels opens the list, a click on the picture closes it, and a key", () => {
    vi.useFakeTimers();
    pointerFocus();
    renderUi(<Harness hintSeen={false} />);
    click(channelsControl());
    expect(listbox()).not.toBeNull();
    fireEvent.pointerLeave(screen.getByTestId("watch-quiet-row"));
    click(screen.getByTestId("watch-picture").firstElementChild as HTMLElement);
    expect(listbox()).toBeNull();
    // A pointer closed it, so focus is left where it was, not handed to Channels.
    expect(document.activeElement).not.toBe(channelsControl());
    keyboardFocus();
    key("ArrowDown");
    expect(restedAfter(6000)).toBe(true);
    expect(tunes()).toEqual(["ch-2"]);
    expect(screen.queryByTestId("ring-actionFocus")).toBeNull();
    requests.mockClear();
  });

  it("gives focus back to Channels when a key closes the list it opened", async () => {
    keyboardFocus();
    renderUi(<Harness />);
    act(() => channelsControl().focus());
    await userEvent.keyboard("{Enter}");
    expect(listbox()).not.toBeNull();
    await userEvent.keyboard("{Escape}");
    expect(listbox()).toBeNull();
    expect(document.activeElement).toBe(channelsControl());
  });

  it("leaves Space to a control the keyboard holds: it presses it and does not also pause", async () => {
    keyboardFocus();
    renderUi(<Harness />);
    act(() => screen.getByRole("button", { name: "Mute (M)" }).focus());
    await userEvent.keyboard(" ");
    expect(requests.mock.calls.map(([request]) => request.type)).toEqual(["mute"]);
    requests.mockClear();
  });

  it("holds awake while the keyboard holds a control", async () => {
    vi.useFakeTimers();
    keyboardFocus();
    renderUi(<Harness hintSeen={false} />);
    act(() => channelsControl().focus());
    act(() => {
      vi.advanceTimersByTime(60_000);
    });
    expect(screen.getByRole("note")).toBeTruthy();
  });

  it("refuses to pause under a cover", () => {
    renderUi(<Harness cover />);
    fireEvent.click(screen.getByTestId("watch-picture").firstElementChild as Element);
    key(" ", document.body);
    expect(requests).not.toHaveBeenCalled();
  });
});

describe("WatchScreen", () => {
  it("flips on Up and Down at once and shows only the lower third", () => {
    renderUi(<Harness />);
    key("ArrowDown");
    key("ArrowDown");
    key("ArrowUp");
    expect(tunes()).toEqual(["ch-2", "ch-3", "ch-2"]);
    expect(listbox()).toBeNull();
    expect(screen.getByTestId("lower-third-title").textContent).toBe("Show 2");
    requests.mockClear();
  });

  it("never calls the tune when the list opens and its cursor moves; Enter tunes it", async () => {
    renderUi(<Harness />);
    await userEvent.click(channelsControl());
    expect(listbox()).not.toBeNull();
    for (const each of ["ArrowDown", "ArrowDown", "ArrowUp", "ArrowRight", "ArrowLeft"]) key(each);
    expect(tunes()).toEqual([]);
    // Changing group put the cursor back on the watched channel.
    key("ArrowDown");
    expect(tunes()).toEqual([]);
    await userEvent.keyboard("{Enter}");
    expect(tunes()).toEqual(["ch-2"]);
    expect(listbox()).not.toBeNull();
    requests.mockClear();
  });

  it("gives focus back to Channels when the list it opened closes", async () => {
    renderUi(<Harness />);
    await userEvent.click(channelsControl());
    key("Escape");
    expect(listbox()).toBeNull();
    expect(document.activeElement).toBe(channelsControl());
  });

  it("hands a key from outside the list to the list's cursor", async () => {
    renderUi(<Harness />);
    await userEvent.click(channelsControl());
    // The pointer moves focus to a quiet-row control; Down still walks the list.
    screen.getByRole("button", { name: "Guide" }).focus();
    key("ArrowDown");
    expect(document.activeElement?.getAttribute("role")).toBe("option");
    expect(tunes()).toEqual([]);
  });

  it("draws one amber ring at a time", async () => {
    keyboardFocus();
    renderUi(<Harness />);
    await userEvent.tab();
    expect(screen.getAllByTestId("ring-actionFocus")).toHaveLength(1);
    await userEvent.keyboard("{ArrowRight}{ArrowRight}");
    expect(screen.getAllByTestId("ring-actionFocus")).toHaveLength(1);
    await userEvent.hover(screen.getByRole("button", { name: "Channels" }));
    expect(screen.queryByTestId("ring-contentPrimary")).toBeNull();
  });

  it("takes Back one step at a time: a menu's page, the menu, the list", async () => {
    keyboardFocus();
    renderUi(<Harness />);
    await userEvent.click(channelsControl());
    key("a", document.body);
    await userEvent.click(screen.getByText("menu audio first"));
    expect(screen.getByText("menu audio inner")).toBeTruthy();
    // Focus is on the page after the click; every key after it is still seen.
    act(() => (document.activeElement as HTMLElement | null)?.blur());
    key("Escape", document.body);
    expect(screen.getByText("menu audio first")).toBeTruthy();
    key("Escape", document.body);
    expect(screen.queryByText(/^menu audio/)).toBeNull();
    expect(listbox()).not.toBeNull();
    // A key closed the menu, so focus is back on its control, with the ring.
    expect(document.activeElement?.getAttribute("aria-label")).toBe("Audio (A)");
    expect(screen.getAllByTestId("ring-actionFocus")).toHaveLength(1);
    act(() => (document.activeElement as HTMLElement | null)?.blur());
    key("Escape", document.body);
    expect(listbox()).toBeNull();
    // A key closed the list that Channels opened: focus is on Channels, with the ring.
    expect(document.activeElement).toBe(channelsControl());
    expect(screen.getAllByTestId("ring-actionFocus")).toHaveLength(1);
    key("Escape", document.body);
    expect(requests).toHaveBeenLastCalledWith({ on: false, type: "fullScreen" });
    requests.mockClear();
  });

  it("asks for full screen on F with the list open", async () => {
    renderUi(<Harness />);
    await userEvent.click(channelsControl());
    key("f");
    expect(requests).toHaveBeenLastCalledWith({ on: false, type: "fullScreen" });
    expect(listbox()).not.toBeNull();
    requests.mockClear();
  });

  it("pauses on a click on the picture, and goes live on End", async () => {
    renderUi(<Harness />);
    const picture = screen.getByTestId("watch-picture").firstElementChild as Element;
    fireEvent.click(picture);
    expect(screen.getByRole("button", { name: "Play (Space)" })).toBeTruthy();
    fireEvent.click(picture);
    key("End", document.body);
    expect(requests.mock.calls.map(([request]) => request.type)).toEqual(["pause", "resume", "goLive"]);
    requests.mockClear();
  });

  it("sends focus to the quiet row when Go live goes from under it", async () => {
    vi.useFakeTimers({ toFake: ["Date"] });
    renderUi(<Harness />);
    key(" ", document.body);
    vi.setSystemTime(Date.now() + 5000);
    key(" ", document.body);
    const goLive = screen.getByRole("button", { name: "Go live" });
    goLive.focus();
    key("End", goLive);
    expect(screen.queryByRole("button", { name: "Go live" })).toBeNull();
    expect(document.activeElement).toBe(channelsControl());
    requests.mockClear();
  });

  it("rests the bare picture after five seconds, and never with the list open", async () => {
    vi.useFakeTimers();
    renderUi(<Harness hintSeen={false} />);
    expect(screen.getByRole("note")).toBeTruthy();
    act(() => {
      vi.advanceTimersByTime(5000);
    });
    expect(screen.queryByRole("note")).toBeNull();
    fireEvent.keyDown(document.body, { key: "m" });
    expect(screen.getByRole("note")).toBeTruthy();
    expect(requests).toHaveBeenLastCalledWith({ type: "mute" });
    fireEvent.click(channelsControl());
    act(() => {
      vi.advanceTimersByTime(60_000);
    });
    expect(listbox()).not.toBeNull();
    requests.mockClear();
  });

  it("says aloud the result under find's cursor", async () => {
    renderUi(<Harness />);
    key("/", document.body);
    await userEvent.type(screen.getByRole("searchbox"), "Station 3");
    expect(document.body.textContent).toContain("3 Station 3, Programme 3");
    await userEvent.type(screen.getByRole("searchbox"), "9");
    expect(document.body.textContent).toContain("No channel or programme matches");
    expect(tunes()).toEqual([]);
  });

  it("draws typed digits in place of the numeral and tunes them after a pause", () => {
    vi.useFakeTimers();
    renderUi(<Harness />);
    fireEvent.keyDown(document.body, { key: "4" });
    expect(screen.getByTestId("lower-third-numeral").textContent).toBe("4");
    act(() => {
      vi.advanceTimersByTime(1600);
    });
    expect(tunes()).toEqual(["ch-4"]);
    requests.mockClear();
  });
});

describe("WatchScreen's back control and what it says aloud", () => {
  it("names the back control beside its arrow, and its press is the caller's", () => {
    const onPress = vi.fn();
    renderUi(<Harness back={{ label: "Back", onPress }} />);
    const back = screen.getByRole("button", { name: "Back" });
    expect(within(back).getByText("Back")).toBeTruthy();
    fireEvent.click(back);
    expect(onPress).toHaveBeenCalledOnce();
    expect(requests).not.toHaveBeenCalled();
  });

  it("takes the keyboard's focus off the back control when the pointer presses another control", () => {
    keyboardFocus();
    renderUi(<Harness back={{ label: "Back", onPress: () => undefined }} />);
    const back = screen.getByRole("button", { name: "Back" });
    act(() => back.focus());
    expect(document.activeElement).toBe(back);
    click(mute());
    expect(document.activeElement).not.toBe(back);
    requests.mockClear();
  });

  it("names the full-screen control for the browser's real state", () => {
    renderUi(<Harness fullScreen={false} />);
    expect(screen.getByRole("button", { name: "Full screen (F)" })).toBeTruthy();
    cleanup();
    renderUi(<Harness fullScreen />);
    expect(screen.getByRole("button", { name: "Leave full screen (F)" })).toBeTruthy();
  });

  // The window is 1280 × 720, all picture. A logo 20% tall, inset 5%, reaches (0.05 + 0.2) × 720 =
  // 180 px in from its edges, over the chrome in its corner.
  it.each([
    ["top-left", "watch-back"],
    ["bottom-left", "watch-lower-third"],
    ["bottom-right", "watch-quiet-row"],
  ] as const)("steps the chrome in the %s corner clear of the logo, and nothing else", (corner, moved) => {
    const back = { label: "Back", onPress: () => undefined };
    const at = () =>
      Object.fromEntries(
        ["watch-back", "watch-lower-third", "watch-quiet-row"].map((id) => [
          id,
          screen.getByTestId(id).style[id === "watch-back" ? "top" : "bottom"],
        ]),
      );
    renderUi(<Harness back={back} />);
    const plain = at();
    cleanup();
    renderUi(<Harness back={back} watermark={{ corner, margin: 0.05, size: 0.2 }} />);
    const cleared = at();
    // The chrome in the logo's corner starts past the logo, from the logo's own edge.
    expect(Number.parseFloat(cleared[moved] ?? "0")).toBeGreaterThan(180);
    for (const id of Object.keys(plain).filter((each) => each !== moved)) expect(cleared[id]).toBe(plain[id]);
  });

  it("gives the back control's place to the open list", () => {
    renderUi(<Harness back={{ label: "Guide", onPress: () => undefined }} />);
    fireEvent.click(channelsControl());
    expect(screen.queryByTestId("watch-back")).toBeNull();
    requests.mockClear();
  });

  it("keeps the screen awake while the pointer rests on the back control", () => {
    vi.useFakeTimers();
    renderUi(<Harness back={{ label: "Guide", onPress: () => undefined }} />);
    fireEvent.pointerEnter(screen.getByTestId("watch-back"));
    act(() => {
      vi.advanceTimersByTime(60_000);
    });
    expect(screen.getByTestId("watch-back").style.opacity).not.toBe("0");
    fireEvent.pointerLeave(screen.getByTestId("watch-back"));
    act(() => {
      vi.advanceTimersByTime(6_000);
    });
    expect(screen.getByTestId("watch-back").style.opacity).toBe("0");
  });

  it("says the lower third once for a channel change, and not as the clock moves on", () => {
    vi.useFakeTimers();
    renderUi(<Harness />);
    const said = () => screen.getByTestId("watch-channel-said").textContent;
    expect(said()).toBe("");
    key("ArrowDown", document.body);
    const first = said();
    expect(first).toContain("Channel 2, Station 2.");
    expect(first).toContain("Show 2");
    act(() => {
      vi.advanceTimersByTime(60_000);
    });
    expect(said()).toBe(first);
    requests.mockClear();
  });
});
