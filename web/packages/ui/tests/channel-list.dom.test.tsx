// @vitest-environment jsdom

import { createChannelLineup } from "@loomarr/core/channel-lineup";
import { type ChannelListState, channelListReduce, createChannelListState } from "@loomarr/core/channel-list";
import { act, cleanup, fireEvent, renderUi, screen, userEvent } from "@loomarr/test-support";
import { declarationsOfElement } from "@loomarr/test-support/css";
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
    await userEvent.keyboard("{ArrowDown}");
    expect(focused()).toBe(row(1));
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
