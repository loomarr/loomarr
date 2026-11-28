// @vitest-environment jsdom

import { idleTune, tuneView } from "@loomarr/core/tune";
import { type WatchRequest, watchKeyOf } from "@loomarr/core/watch-screen";
import { act, cleanup, fireEvent, renderUi, screen, userEvent } from "@loomarr/test-support";
import { useEffect } from "react";
import { Text } from "react-native";
import { afterEach, describe, expect, it, vi } from "vitest";

import { useWatchScreen, WatchScreen } from "../index";

const nativeMatches = Element.prototype.matches;
afterEach(() => {
  cleanup();
  vi.useRealTimers();
  Element.prototype.matches = nativeMatches;
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

// The caller's part: the model, the document's keys, and slots for a menu.
const Harness = ({ hintSeen = true }: { hintSeen?: boolean }) => {
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
  return (
    <WatchScreen
      layout="full"
      model={model}
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
    renderUi(<Harness />);
    await userEvent.click(channelsControl());
    key("a", document.body);
    await userEvent.click(screen.getByText("menu audio first"));
    expect(screen.getByText("menu audio inner")).toBeTruthy();
    key("Escape", document.body);
    expect(screen.getByText("menu audio first")).toBeTruthy();
    key("Escape", document.body);
    expect(screen.queryByText(/^menu audio/)).toBeNull();
    expect(listbox()).not.toBeNull();
    key("Escape", document.body);
    expect(listbox()).toBeNull();
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
