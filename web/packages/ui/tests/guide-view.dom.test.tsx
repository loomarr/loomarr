// @vitest-environment jsdom

import { createChannelLineup } from "@loomarr/core/channel-lineup";
import {
  createGuideViewState,
  type GuideViewContext,
  type GuideViewState,
  guideViewReduce,
} from "@loomarr/core/guide-view";
import { cleanup, renderUi, screen, userEvent } from "@loomarr/test-support";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { GuideView, type GuideViewEntry, guideViewChannels, type StripAiring } from "../index";

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
  nowMs: 0.5 * HOUR,
  untilMs: 6 * HOUR,
  widthMs: 3 * HOUR,
};

const onTune = vi.fn();
const onClose = vi.fn();

const Harness = ({ layout = "desktop" as const }) => {
  const [state, setState] = useState<GuideViewState>(() =>
    createGuideViewState({ watchedId: "coast" }, context),
  );
  const [message, setMessage] = useState<string>();
  return (
    <GuideView
      autoFocus
      context={context}
      layout={layout}
      lowerThird={<span>Lower third</span>}
      message={message}
      onEvent={(event) => {
        const step = guideViewReduce(state, event, context);
        setState(step.state);
        setMessage(step.message);
        if (step.tune) onTune(step.tune);
        if (step.close) onClose();
      }}
      picture={<span>Picture</span>}
      state={state}
    />
  );
};

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
    await user.click(screen.getByRole("button", { name: "T1 Marathon" }));
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
