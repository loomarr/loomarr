// @vitest-environment jsdom

import { createChannelLineup } from "@loomarr/core/channel-lineup";
import {
  createGuideViewState,
  type GuideViewContext,
  type GuideViewEvent,
  type GuideViewState,
  guideViewReduce,
} from "@loomarr/core/guide-view";
import { act, cleanup, fireEvent, renderUi, screen, userEvent } from "@loomarr/test-support";
import { createRef, type Ref, useState } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { GuideView, type GuideViewEntry, type GuideViewHandle, guideViewChannels } from "../guide";
import type { StripAiring } from "../index";

afterEach(cleanup);

const HOUR = 3_600_000;
const formatTime = (ms: number) => `T${ms / HOUR}`;
const airing = (from: number, to: number, title: string, fields: Partial<StripAiring> = {}): StripAiring => ({
  kind: "program",
  scheduleBlockId: `${title}-${from}`,
  startMs: from * HOUR,
  stopMs: to * HOUR,
  title,
  ...fields,
});

const channels = guideViewChannels([
  {
    airings: [
      airing(-1, 0, "Earlier film"),
      airing(0, 1, "Opening"),
      airing(1, 2, "Pilot", { series: "Diner" }),
      airing(2, 2.5, "Second", { series: "Diner" }),
      airing(2.5, 5, "Closer"),
    ],
    decade: 1990,
    id: "coast",
    name: "Coast 90s",
    number: 7,
    playable: true,
  },
  {
    airings: [airing(0, 1, "Quiz"), airing(1, 4, "Marathon")],
    id: "game",
    name: "Game Night",
    number: 9,
    playable: true,
  },
  { airings: [], id: "quiet", name: "Quiet", number: 11, playable: true },
]);

const lineup = createChannelLineup<GuideViewEntry>({ channels });
const context: GuideViewContext<GuideViewEntry> = {
  formatTime,
  lineup,
  hourStartMs: 0,
  nowMs: 0.5 * HOUR,
  untilMs: 6 * HOUR,
  widthMs: 3 * HOUR,
};

const onTune = vi.fn();
const onClose = vi.fn();

interface HarnessProps {
  layout?: "desktop";
  watchedId?: string;
  group?: "decade" | "favourites";
  ctx?: GuideViewContext<GuideViewEntry>;
  autoFocus?: boolean;
  handle?: Ref<GuideViewHandle>;
  /** Hands out the way an outside event, such as another screen's tune, reaches the model. */
  outside?: (send: (event: GuideViewEvent) => void) => void;
}

const Harness = ({
  autoFocus = true,
  ctx = context,
  group,
  handle,
  layout = "desktop",
  outside,
  watchedId = "coast",
}: HarnessProps) => {
  const [state, setState] = useState<GuideViewState>(() => createGuideViewState({ group, watchedId }, ctx));
  const [message, setMessage] = useState<string>();
  const send = (event: GuideViewEvent) => {
    const step = guideViewReduce(state, event, ctx);
    setState(step.state);
    setMessage(step.message);
    if (step.tune) onTune(step.tune);
    if (step.close) onClose();
  };
  outside?.(send);
  return (
    <GuideView
      autoFocus={autoFocus}
      context={ctx}
      handle={handle}
      layout={layout}
      lowerThird={<span>Lower third</span>}
      message={message}
      onEvent={send}
      picture={<span>Picture</span>}
      state={state}
    />
  );
};

// The browser's own test of keyboard focus: true while a key was the last thing used.
const nativeMatches = Element.prototype.matches;
let lastWasKey = true;
const key = () => {
  lastWasKey = true;
};
const point = () => {
  lastWasKey = false;
};
beforeEach(() => {
  lastWasKey = true;
  document.addEventListener("keydown", key, true);
  document.addEventListener("pointerdown", point, true);
  Element.prototype.matches = new Proxy(nativeMatches, {
    apply: (original, element, args: [string]) =>
      args[0] === ":focus-visible" ? lastWasKey : Reflect.apply(original, element, args),
  });
});
afterEach(() => {
  document.removeEventListener("keydown", key, true);
  document.removeEventListener("pointerdown", point, true);
  Element.prototype.matches = nativeMatches;
});

const rings = (tone: "actionFocus" | "contentPrimary") =>
  document.querySelectorAll(
    `[data-testid="guide-view-ring-${tone}"], [data-testid="film-strip-ring-${tone}"]`,
  ).length;

/** A watched channel's context with these airings, and a second channel beside it. */
const withAirings = (airings: StripAiring[]): GuideViewContext<GuideViewEntry> => ({
  ...context,
  lineup: createChannelLineup<GuideViewEntry>({
    channels: guideViewChannels([{ airings, id: "coast", name: "Coast 90s", number: 7, playable: true }]),
  }),
});

const focused = () => document.activeElement?.getAttribute("aria-label");

describe("GuideView", () => {
  it("opens on what is on now on the watched channel, beside the playing picture", () => {
    renderUi(<Harness />);
    expect(focused()).toContain("Opening");
    expect(screen.getByText("Lower third")).toBeTruthy();
    expect(screen.getByText("Playing 7 Coast 90s")).toBeTruthy();
    expect(screen.getByRole("group", { name: "11 Quiet" }).textContent).toContain("Nothing scheduled");
  });

  it("looks without playing: the header follows the cursor and the caption stays", async () => {
    const user = userEvent.setup();
    renderUi(<Harness />);
    await user.keyboard("{ArrowDown}");
    expect(focused()).toContain("Quiz");
    expect(screen.getByText("Looking at Game Night · on now until T1")).toBeTruthy();
    expect(screen.getByText("Playing 7 Coast 90s")).toBeTruthy();
  });

  it("keeps one amber ring as the cursor crosses the strip and the compact rows", async () => {
    const user = userEvent.setup();
    renderUi(<Harness />);
    const amber = () => document.querySelectorAll('[data-testid$="ring-actionFocus"]').length;
    for (const key of ["{ArrowRight}", "{ArrowDown}", "{ArrowUp}"]) {
      await user.keyboard(key);
      expect(amber()).toBe(1);
    }
    expect(focused()).toContain("Diner");
  });

  it("says when a later programme starts on Enter, tunes what is on now, and returns on Escape", async () => {
    const user = userEvent.setup();
    renderUi(<Harness />);
    await user.keyboard("{ArrowRight}{Enter}");
    expect(screen.getByRole("status").textContent).toBe(
      "Diner starts at T1. Enter on what is on now watches Coast 90s",
    );
    expect(onTune).not.toHaveBeenCalled();
    await user.keyboard("{ArrowLeft}{ArrowDown}{Enter}");
    expect(onTune).toHaveBeenCalledTimes(1);
    expect(onTune).toHaveBeenCalledWith("game");
    await user.keyboard("{Escape}");
    expect(onClose).toHaveBeenCalled();
  });

  it("only looks on a click on a later programme, and tunes on a click on a row's channel", async () => {
    const user = userEvent.setup();
    onTune.mockClear();
    renderUi(<Harness />);
    await user.click(screen.getByRole("button", { name: /^T1 Marathon/ }));
    expect(onTune).not.toHaveBeenCalled();
    expect(screen.getByRole("status").textContent).toBe("");
    expect(screen.getByText("Looking at Game Night · T1 to T4")).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "9 Game Night, watch channel" }));
    expect(onTune).toHaveBeenCalledWith("game");
  });

  it("counts a run inside the span, and not one its edge cuts", async () => {
    const user = userEvent.setup();
    renderUi(<Harness />);
    expect(screen.getByRole("button", { name: /Diner/ }).textContent).toContain("2 episodes");
    // Later puts the span's start inside the run.
    await user.click(screen.getByRole("button", { name: "Later" }));
    await user.click(screen.getByRole("button", { name: "Later" }));
    expect(screen.getByRole("button", { name: /Diner/ }).textContent).not.toContain("episodes");
  });

  it("pages with Earlier and Later, and says where the schedule ends", async () => {
    const user = userEvent.setup();
    renderUi(<Harness />);
    const earlier = () => screen.getByRole("button", { name: "Earlier" });
    expect(earlier().getAttribute("aria-disabled") ?? String(earlier().hasAttribute("disabled"))).toBe(
      "true",
    );
    for (let page = 0; page < 3; page += 1) await user.click(screen.getByRole("button", { name: "Later" }));
    expect(screen.getByText("The schedule ends at T6")).toBeTruthy();
  });

  it("reaches the group tabs from the first row and says a group is empty", async () => {
    const user = userEvent.setup();
    renderUi(<Harness />);
    await user.keyboard("{ArrowUp}");
    expect(focused()).toBe("All");
    await user.keyboard("{ArrowLeft}{Enter}");
    expect(screen.getByText("No favourites yet.")).toBeTruthy();
    await user.keyboard("{ArrowDown}");
    expect(focused()).toContain("Opening");
  });

  it("keeps the keys working when the watched channel has nothing scheduled", async () => {
    const user = userEvent.setup();
    renderUi(<Harness watchedId="quiet" />);
    expect(focused()).toBe("11 Quiet, watch channel");
    await user.keyboard("{ArrowUp}");
    expect(focused()).toContain("Quiz");
  });

  const part = (from: number, to: number, title: string) => airing(from, to, title, { series: "Diner" });

  it.each([
    [
      "right",
      [airing(0, 1, "Opening"), part(1, 2, "One"), part(2, 3.5, "Two"), part(3.5, 5, "Three")],
      "{ArrowRight}",
    ],
    [
      "left",
      [part(-1, -0.5, "One"), part(-0.5, 0.25, "Two"), part(0.25, 1, "Three"), airing(1, 5, "Closer")],
      "{ArrowRight}{ArrowLeft}",
    ],
  ])("keeps the cursor on a run that crosses the span's %s edge", async (edge, airings, keys) => {
    const user = userEvent.setup();
    onClose.mockClear();
    renderUi(<Harness ctx={withAirings(airings)} />);
    await user.keyboard(keys);
    expect(focused()).toContain("Diner");
    expect(rings("actionFocus")).toBe(1);
    await user.keyboard("{Enter}");
    if (edge === "right") {
      expect(screen.getByRole("status").textContent).toContain("Diner starts at T1");
      expect(onClose).not.toHaveBeenCalled();
    } else expect(onClose).toHaveBeenCalled();
  });

  it("is amber only while the keys drive, and never twice", async () => {
    const user = userEvent.setup();
    renderUi(<Harness />);
    await user.click(screen.getByRole("button", { name: /^T1 Marathon/ }));
    expect([rings("actionFocus"), rings("contentPrimary")]).toEqual([0, 1]);
    await user.keyboard("{ArrowUp}");
    expect(rings("actionFocus")).toBe(1);
    // Tab to the controls above the rows: their ring is theirs alone.
    for (const name of ["All", "Later"]) {
      act(() => screen.getByRole(name === "All" ? "tab" : "button", { name }).focus());
      expect(rings("actionFocus")).toBe(0);
    }
  });

  it("keeps the keys after Show all channels, by a key or by a pointer", async () => {
    const user = userEvent.setup();
    renderUi(<Harness group="favourites" />);
    const showAll = () => screen.getByRole("button", { name: "Show all channels" });
    act(() => showAll().focus());
    await user.keyboard("{Enter}");
    expect(focused()).toContain("Opening");
    expect(rings("actionFocus")).toBe(1);

    cleanup();
    renderUi(<Harness group="favourites" />);
    await user.click(showAll());
    expect(document.activeElement).not.toBe(document.body);
    expect(rings("actionFocus")).toBe(0);
    await user.keyboard("{ArrowDown}");
    expect(focused()).toContain("Quiz");
  });

  it("follows an outside tune that takes the cursor's row from the group", async () => {
    const user = userEvent.setup();
    let send: (event: GuideViewEvent) => void = () => undefined;
    renderUi(
      <Harness
        group="favourites"
        outside={(next) => {
          send = next;
        }}
      />,
    );
    expect(focused()).toContain("Opening");
    act(() => send({ channelId: "game", type: "tuned" }));
    expect(focused()).toContain("Quiz");
    await user.keyboard("{Enter}");
    expect(onClose).toHaveBeenCalled();
  });

  it("takes keys a caller feeds it, and leaves one already taken", () => {
    const handle = createRef<GuideViewHandle>();
    renderUi(<Harness autoFocus={false} handle={handle} />);
    const down = (defaultPrevented: boolean) => ({
      altKey: false,
      ctrlKey: false,
      defaultPrevented,
      key: "ArrowDown",
      metaKey: false,
      preventDefault: () => undefined,
    });
    act(() => handle.current?.onKeyDown(down(true)));
    expect(screen.getByText("Lower third")).toBeTruthy();
    act(() => handle.current?.onKeyDown(down(false)));
    expect(focused()).toContain("Quiz");
  });

  it("counts Enter after a stray pointer press as the keys'", async () => {
    const user = userEvent.setup();
    renderUi(<Harness />);
    await user.keyboard("{ArrowRight}");
    fireEvent.pointerDown(screen.getByText("Playing 7 Coast 90s"));
    await user.keyboard("{Enter}");
    expect(screen.getByRole("status").textContent).toContain("Diner starts at T1");
  });

  it("names each block with its channel, in the strip and in a compact row", () => {
    renderUi(<Harness />);
    expect(screen.getByRole("button", { name: /Opening, channel 7, Coast 90s$/ })).toBeTruthy();
    expect(screen.getByRole("button", { name: "T1 Marathon, channel 9, Game Night" })).toBeTruthy();
  });

  it("heads By decade with its decades in their tones, and dims the aired part of what is on now", () => {
    const lineupOf = createChannelLineup<GuideViewEntry>({
      channels: channels.map((channel) => (channel.id === "game" ? { ...channel, decade: 1980 } : channel)),
    });
    renderUi(<Harness ctx={{ ...context, lineup: lineupOf }} group="decade" />);
    const headings = screen.getAllByRole("heading").map((heading) => heading.textContent);
    expect(headings).toEqual(["1980s", "1990s"]);
    const tone = (words: string) =>
      getComputedStyle(screen.getByText(words)).color + screen.getByText(words).className;
    expect(tone("1980s")).not.toBe(tone("1990s"));
    // Half of the hour has aired on the compact row.
    expect(getComputedStyle(screen.getByTestId("guide-view-aired")).width).toBe("50%");
  });

  it("moves focus, and so the scroll, for the keys and never for a pointer", async () => {
    const user = userEvent.setup();
    renderUi(<Harness autoFocus={false} />);
    const focus = vi.spyOn(HTMLElement.prototype, "focus");
    const target = screen.getByRole("button", { name: /^T1 Marathon/ });
    await user.click(target);
    expect(focus.mock.contexts.filter((element) => element !== target)).toEqual([]);
    focus.mockClear();
    await user.keyboard("{ArrowUp}");
    expect(focus).toHaveBeenCalled();
    focus.mockRestore();
  });

  it("draws a single-channel lineup", () => {
    renderUi(<Harness ctx={withAirings([airing(0, 5, "Opening")])} />);
    expect(screen.getAllByRole("group", { name: /Coast 90s/ })).toHaveLength(1);
    expect(focused()).toContain("Opening");
  });

  it("reads the ruler on the viewer's hours in a zone off by part of one", () => {
    const hourStartMs = 0.25 * HOUR;
    const clock = (ms: number) => {
      const minutes = Math.round((ms - hourStartMs) / 60_000);
      return `${Math.floor(minutes / 60)}:${String(((minutes % 60) + 60) % 60).padStart(2, "0")}`;
    };
    renderUi(<Harness ctx={{ ...context, formatTime: clock, hourStartMs }} />);
    for (const time of ["0:00", "0:30", "1:00"]) expect(screen.getAllByText(time).length).toBeGreaterThan(0);
  });

  it("says it is loading before the schedule arrives", () => {
    renderUi(<GuideView layout="television" loading picture={<span>Picture</span>} />);
    expect(screen.getByText("Loading channels")).toBeTruthy();
    expect(screen.getByText("Picture")).toBeTruthy();
  });

  it("has no Earlier or Later on a television", () => {
    renderUi(<Harness layout={"television" as never} />);
    expect(screen.queryByRole("button", { name: "Later" })).toBeNull();
  });
});
