// @vitest-environment jsdom

import { createChannelLineup } from "@loomarr/core/channel-lineup";
import { type ChannelListState, channelListReduce, createChannelListState } from "@loomarr/core/channel-list";
import { act, cleanup, fireEvent, renderUi, screen, userEvent } from "@loomarr/test-support";
import { declarationsOfElement, declarationsOfTree } from "@loomarr/test-support/css";
import { useState } from "react";
import { Text } from "react-native";
import { afterEach, describe, expect, it, vi } from "vitest";

import { ChannelList, type ChannelListProps } from "../index";

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

const lineup = createChannelLineup({
  channels: [1, 2, 3, 4].map((number) => ({
    decade: number < 3 ? 1980 : 1990,
    id: `ch-${number}`,
    name: `Station ${number}`,
    number,
    onNow: `Programme ${number}`,
    playable: true,
  })),
});

const tuned = vi.fn();

// The caller's part: run the model, tune what it asks, and answer with the new watched channel.
const Harness = ({
  start,
  ...props
}: { start?: Partial<ChannelListState> } & Partial<ChannelListProps<(typeof lineup.channels)[number]>>) => {
  const [state, setState] = useState<ChannelListState>({
    ...createChannelListState({ open: true, watchedId: "ch-2" }),
    ...start,
  });
  return (
    <>
      <ChannelList
        detail={(channel) => (channel.number === 3 ? { inBreak: true } : { minutesLeft: 12 })}
        lineup={lineup}
        onEvent={(event) =>
          setState((now) => {
            const step = channelListReduce(now, event, lineup);
            if (!step.tune) return step.state;
            tuned(step.tune);
            return channelListReduce(step.state, { channelId: step.tune, type: "tuned" }, lineup).state;
          })
        }
        renderPinned={(channel) => <Text>{`Pinned ${channel.number}`}</Text>}
        restOffset={120}
        state={state}
        {...props}
      />
      <output>{state.open ? "open" : "closed"}</output>
    </>
  );
};

// A row's name begins with its number; the pinned one begins with the caller's drawing.
const row = (number: number) => screen.getByRole("option", { name: new RegExp(`^(Pinned )?${number} `) });
// The amber ring is the focused element's, so focus is where the ring is.
const focused = () => document.activeElement;

describe("ChannelList", () => {
  it("lists every row with its title and time, and marks the watched channel in its pinned slot", () => {
    renderUi(<Harness />);
    expect(row(1).textContent).toBe("1 Station 1 Programme 1 12 min left ");
    expect(row(3).textContent).toBe("3 Station 3 Programme 3 in a break ");
    expect(row(2).getAttribute("aria-selected")).toBe("true");
    expect(row(2).textContent).toBe("Pinned 2watching");
    expect(screen.getByTestId("channel-list-pinned").contains(row(2))).toBe(true);
    expect(screen.getAllByRole("option")).toHaveLength(4);
  });

  it("moves one amber ring with the keys, through the pinned channel and up to the tabs, without tuning", async () => {
    tuned.mockClear();
    renderUi(<Harness autoFocus start={{ cursor: { channelId: "ch-2", kind: "row" } }} />);
    expect(focused()).toBe(row(2));
    await userEvent.keyboard("{ArrowUp}");
    expect(focused()).toBe(row(1));
    await userEvent.keyboard("{ArrowUp}");
    expect(focused()).toBe(screen.getByRole("tab", { name: "All" }));
    await userEvent.keyboard("{ArrowRight}");
    expect(focused()).toBe(screen.getByRole("tab", { name: "Recent" }));
    // Down comes back to the watched channel.
    await userEvent.keyboard("{ArrowDown}");
    expect(focused()).toBe(row(2));
    expect(tuned).not.toHaveBeenCalled();
  });

  it("tunes on Enter, and closes on Escape", async () => {
    tuned.mockClear();
    renderUi(<Harness autoFocus start={{ cursor: { channelId: "ch-2", kind: "row" } }} />);
    await userEvent.keyboard("{ArrowDown}{Enter}");
    expect(tuned).toHaveBeenCalledExactlyOnceWith("ch-3");
    expect(screen.getByRole("status").textContent).toBe("open");
    await userEvent.keyboard("{Escape}");
    expect(screen.getByRole("status").textContent).toBe("closed");
  });

  it("ignores a second click on the same spot until the pointer has moved", () => {
    tuned.mockClear();
    renderUi(<Harness />);
    fireEvent.click(row(3), { clientX: 40, clientY: 60 });
    fireEvent.click(row(4), { clientX: 40, clientY: 60 });
    expect(tuned.mock.calls).toEqual([["ch-3"]]);
    fireEvent.pointerMove(row(4), { clientX: 80, clientY: 60 });
    fireEvent.click(row(4), { clientX: 80, clientY: 60 });
    expect(tuned.mock.calls).toEqual([["ch-3"], ["ch-4"]]);
  });

  it("never tunes or moves the cursor on a wheel", () => {
    tuned.mockClear();
    renderUi(<Harness autoFocus start={{ cursor: { channelId: "ch-2", kind: "row" } }} />);
    fireEvent.wheel(screen.getByTestId("channel-list-scroller"), { deltaY: 400 });
    expect(tuned).not.toHaveBeenCalled();
    expect(focused()).toBe(row(2));
  });

  it("draws the amber ring for keys alone: not on a row focused after a click, nor on a click", async () => {
    // A browser answers `:focus-visible` with false for focus that follows a click, as when a
    // pointer opens the list and it takes focus onto its cursor, and true for focus from a key.
    const native = Element.prototype.matches;
    let keyboard = false;
    Element.prototype.matches = new Proxy(native, {
      apply: (original, element, args: [string]) =>
        args[0] === ":focus-visible" ? keyboard : Reflect.apply(original, element, args),
    });
    try {
      renderUi(<Harness autoFocus start={{ cursor: { channelId: "ch-2", kind: "row" } }} />);
      expect(focused()).toBe(row(2));
      expect(screen.queryByTestId("ring-actionFocus")).toBeNull();
      await userEvent.click(row(3));
      expect(screen.queryByTestId("ring-actionFocus")).toBeNull();
      // A key brings focus back to the list's cursor, with the amber ring.
      keyboard = true;
      act(() => row(3).focus());
      expect(screen.getAllByTestId("ring-actionFocus")).toHaveLength(1);
      await userEvent.keyboard("{ArrowDown}");
      expect(focused()).toBe(row(4));
      expect(screen.getAllByTestId("ring-actionFocus")).toHaveLength(1);
    } finally {
      Element.prototype.matches = native;
    }
  });

  it("draws no amber ring on a tab or a button focused after a click, and one for a key", () => {
    const native = Element.prototype.matches;
    let keyboard = false;
    Element.prototype.matches = new Proxy(native, {
      apply: (original, element, args: [string]) =>
        args[0] === ":focus-visible" ? keyboard : Reflect.apply(original, element, args),
    });
    try {
      renderUi(<Harness />);
      for (const control of [
        screen.getByRole("tab", { name: "Recent" }),
        screen.getByRole("button", { name: "Find a channel" }),
      ]) {
        act(() => control.focus());
        expect(screen.queryByTestId("ring-actionFocus")).toBeNull();
      }
      keyboard = true;
      act(() => screen.getByRole("tab", { name: "All" }).focus());
      expect(screen.getAllByTestId("ring-actionFocus")).toHaveLength(1);
      expect(screen.getByRole("tab", { name: "All", selected: true })).toBeTruthy();
    } finally {
      Element.prototype.matches = native;
    }
  });

  it("never takes focus from a pointer, so a click draws no ring", async () => {
    renderUi(<Harness />);
    await userEvent.click(row(3));
    expect(focused()).toBe(document.body);
  });

  it("reports looking after a rest, ends it at once, and closes on no timer", () => {
    vi.useFakeTimers();
    const onLook = vi.fn();
    renderUi(<Harness onLook={onLook} />);
    fireEvent.pointerEnter(row(3), { pointerType: "mouse" });
    expect(onLook).not.toHaveBeenCalled();
    act(() => vi.advanceTimersByTime(200));
    expect(onLook).toHaveBeenLastCalledWith("ch-3");
    fireEvent.pointerLeave(row(3), { pointerType: "mouse" });
    expect(onLook).toHaveBeenLastCalledWith(undefined);
    act(() => vi.advanceTimersByTime(60 * 60_000));
    expect(screen.getByRole("status").textContent).toBe("open");
  });

  it("opens find with a slash, offers every channel on no match, and draws none on a television", async () => {
    renderUi(<Harness autoFocus start={{ cursor: { channelId: "ch-2", kind: "row" } }} />);
    await userEvent.keyboard("/");
    await userEvent.type(screen.getByRole("searchbox"), "zzz");
    expect(screen.getByText("No channel or programme matches “zzz”.")).toBeTruthy();
    await userEvent.click(screen.getByRole("button", { name: "Show all channels" }));
    expect(screen.queryByRole("searchbox")).toBeNull();
    expect(screen.getAllByRole("option")).toHaveLength(4);
    cleanup();
    renderUi(<Harness layout="television" start={{ television: true }} />);
    expect(screen.queryByRole("button", { name: "Find a channel" })).toBeNull();
  });

  it("draws a plain highlight under a pointer, and takes it away", () => {
    renderUi(<Harness />);
    const fill = () => declarationsOfTree(row(3));
    expect(fill()).not.toContain("var(--surfaceElevated)");
    fireEvent.pointerEnter(row(3), { pointerType: "mouse" });
    expect(fill()).toContain("var(--surfaceElevated)");
    expect(row(3).getAttribute("aria-selected")).toBe("false");
    fireEvent.pointerLeave(row(3), { pointerType: "mouse" });
    expect(fill()).not.toContain("var(--surfaceElevated)");
  });

  it("draws find's cursor on the result it rests on, keeps focus in the query and tunes it on Enter", async () => {
    tuned.mockClear();
    renderUi(<Harness autoFocus start={{ cursor: { channelId: "ch-2", kind: "row" } }} />);
    await userEvent.keyboard("/");
    const query = screen.getByRole("searchbox");
    // Every channel's title matches.
    await userEvent.type(query, "Programme");
    const fills = () =>
      [1, 2, 3, 4].filter((n) => declarationsOfTree(row(n)).includes("var(--surfaceElevated)"));
    expect(fills()).toEqual([1]);
    await userEvent.keyboard("{ArrowDown}");
    expect(focused()).toBe(query);
    expect(fills()).toEqual([2]);
    // The second result is channel 2, the one on screen, so step on to the third.
    await userEvent.keyboard("{ArrowDown}");
    expect(fills()).toEqual([3]);
    // The query's ring is the only amber one.
    expect(declarationsOfElement(row(3).parentElement as HTMLElement)).not.toContain("var(--actionFocus)");
    await userEvent.keyboard("{Enter}");
    expect(tuned).toHaveBeenCalledExactlyOnceWith("ch-3");
    expect(screen.queryByRole("searchbox")).toBeNull();
    expect(screen.getByRole("status").textContent).toBe("open");
    expect(focused()).toBe(row(3));
  });

  it("marks one element as watched among find's results, and leaves the list open on choosing it", async () => {
    tuned.mockClear();
    renderUi(<Harness start={{ query: "Station 2" }} />);
    expect(screen.getAllByRole("option", { selected: true })).toHaveLength(1);
    expect(screen.getAllByText("watching")).toHaveLength(1);
    await userEvent.click(screen.getByRole("option", { selected: true }));
    expect(tuned).not.toHaveBeenCalled();
    expect(screen.queryByRole("searchbox")).toBeNull();
    expect(screen.getByRole("status").textContent).toBe("open");
  });

  it("takes Escape one step at a time, and goes to the ends on Home and End", async () => {
    renderUi(<Harness autoFocus start={{ cursor: { channelId: "ch-2", kind: "row" } }} />);
    await userEvent.keyboard("{End}");
    expect(focused()).toBe(row(4));
    await userEvent.keyboard("{Home}");
    expect(focused()).toBe(row(1));
    await userEvent.keyboard("{ArrowUp}{ArrowRight}{Enter}");
    expect(focused()).toBe(screen.getByRole("tab", { name: "Recent" }));
    await userEvent.keyboard("{Escape}");
    expect(focused()).toBe(row(2));
    expect(screen.getByRole("status").textContent).toBe("open");
    await userEvent.keyboard("/");
    await userEvent.keyboard("{Escape}");
    expect(screen.queryByRole("searchbox")).toBeNull();
    expect(screen.getByRole("status").textContent).toBe("open");
    await userEvent.keyboard("{Escape}");
    expect(screen.getByRole("status").textContent).toBe("closed");
  });

  it("reaches the tabs with Up from the first row on a television", async () => {
    renderUi(
      <Harness
        autoFocus
        layout="television"
        start={{ cursor: { channelId: "ch-1", kind: "row" }, television: true }}
      />,
    );
    await userEvent.keyboard("{ArrowUp}");
    expect(focused()).toBe(screen.getByRole("tab", { name: "All" }));
  });

  it("keeps its notice out of the options", async () => {
    renderUi(<Harness />);
    await userEvent.click(screen.getByRole("tab", { name: "Recent" }));
    const notice = screen.getByText("Channels you watch collect here.");
    expect(notice.closest('[role="listbox"]')).toBeNull();
  });

  const filled = (element: HTMLElement) => declarationsOfTree(element).includes("var(--surfaceElevated)");

  it("draws the cursor from the moment find opens, before anything is typed", async () => {
    tuned.mockClear();
    renderUi(<Harness autoFocus start={{ cursor: { channelId: "ch-2", kind: "row" } }} />);
    await userEvent.keyboard("/");
    expect(filled(row(2))).toBe(true);
    await userEvent.keyboard("{ArrowDown}");
    expect(focused()).toBe(screen.getByRole("searchbox"));
    expect(filled(row(3))).toBe(true);
    expect(filled(row(2))).toBe(false);
    await userEvent.keyboard("{Enter}");
    expect(tuned).toHaveBeenCalledExactlyOnceWith("ch-3");
  });

  it("leaves Enter to an input method while it is composing", () => {
    tuned.mockClear();
    renderUi(<Harness start={{ cursor: { channelId: "ch-3", kind: "row" }, query: "Station" }} />);
    fireEvent.keyDown(screen.getByRole("searchbox"), { isComposing: true, key: "Enter" });
    expect(tuned).not.toHaveBeenCalled();
    fireEvent.keyDown(screen.getByRole("searchbox"), { key: "Enter" });
    expect(tuned).toHaveBeenCalledExactlyOnceWith("ch-3");
  });

  it("leaves the caret's keys to the query", async () => {
    tuned.mockClear();
    renderUi(<Harness autoFocus start={{ cursor: { channelId: "ch-2", kind: "row" } }} />);
    await userEvent.keyboard("/");
    const query = screen.getByRole<HTMLInputElement>("searchbox");
    await userEvent.type(query, "Programme");
    await userEvent.keyboard("{Home}");
    expect(query.selectionStart).toBe(0);
    await userEvent.keyboard("{ArrowRight}{ArrowRight}{ArrowLeft}");
    expect(query.selectionStart).toBe(1);
    expect(filled(row(1))).toBe(true);
    await userEvent.keyboard("{End}");
    expect(query.selectionStart).toBe("Programme".length);
    expect(filled(row(1))).toBe(true);
    expect(screen.getAllByRole("option")).toHaveLength(5);
    expect(tuned).not.toHaveBeenCalled();
  });

  it("lets a highlight go when the list scrolls or changes under the pointer", async () => {
    renderUi(<Harness />);
    fireEvent.pointerEnter(row(3), { pointerType: "mouse" });
    expect(filled(row(3))).toBe(true);
    fireEvent.scroll(screen.getByTestId("channel-list-scroller"));
    expect(filled(row(3))).toBe(false);
    fireEvent.pointerEnter(row(4), { pointerType: "mouse" });
    await userEvent.click(screen.getByRole("tab", { name: "By decade" }));
    expect(filled(row(4))).toBe(false);
  });

  it("draws no highlight on a row a press would not answer, until the pointer moves", () => {
    renderUi(<Harness />);
    fireEvent.click(row(3), { clientX: 40, clientY: 60 });
    fireEvent.pointerEnter(row(4), { pointerType: "mouse" });
    expect(filled(row(4))).toBe(false);
    fireEvent.pointerMove(row(4), { clientX: 80, clientY: 60 });
    expect(filled(row(4))).toBe(true);
  });

  it("says what an empty Recent is for, and heads By decade with its decades", async () => {
    renderUi(<Harness />);
    await userEvent.click(screen.getByRole("tab", { name: "Recent" }));
    expect(screen.getByText("Channels you watch collect here.")).toBeTruthy();
    await userEvent.click(screen.getByRole("tab", { name: "By decade" }));
    // The headings are drawn for the eye; each row says its own decade.
    const headings = [...document.querySelectorAll('[aria-hidden="true"]')].map((each) => each.textContent);
    expect(headings.filter(Boolean)).toEqual(["1980s", "1990s"]);
  });

  it.each(["dark", "light"] as const)(
    "draws each heading in its decade's tint in the %s theme, outside any row's fill",
    (theme) => {
      renderUi(<Harness start={{ group: "decade" }} />, { theme });
      for (const decade of ["1980", "1990"]) {
        const heading = screen.getByText(`${decade}s`, { selector: '[aria-hidden="true"] *' });
        expect(declarationsOfElement(heading)).toContain(`var(--decadeTint${decade})`);
        expect(heading.closest('[role="option"]')).toBeNull();
        expect(screen.getByTestId("channel-list-pinned").contains(heading)).toBe(false);
      }
    },
  );
});
