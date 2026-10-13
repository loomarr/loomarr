import { createGuideController, type GuideLayout, type GuideSelection } from "@loomarr/core/guide";
import type { GuideFocusTarget } from "@loomarr/ui";
import { describe, expect, it, vi } from "vitest";

import { createTvGuideFocusRegistry, tvGuideTimeEdge } from "../index";

// The served window decides every programme, so a paged window is told apart by its ids.
const windowGuide = ({ from, to }: { from: number; to: number }): GuideLayout["source"] => {
  const half = from + (to - from) / 2;
  return {
    channels: [
      {
        airings: [
          {
            kind: "program",
            scheduleBlockId: `split-first-${from}`,
            startMs: from,
            stopMs: half,
            title: "One",
          },
          {
            kind: "program",
            scheduleBlockId: `split-second-${from}`,
            startMs: half,
            stopMs: to,
            title: "Two",
          },
        ],
        channelId: "split",
        name: "Classic Animation",
        number: 77,
        pendingCount: 0,
        status: "live",
      },
      {
        airings: [{ kind: "program", scheduleBlockId: "spanning", startMs: from, stopMs: to, title: "Long" }],
        channelId: "spanning",
        name: "Science Fiction",
        number: 120,
        pendingCount: 0,
        status: "live",
      },
    ],
    fromMs: from,
    toMs: to,
  };
};

const live = { from: 1_000, to: 5_000 };
const nowMs = 2_500;

// guide-tv's cell target: the cell's midpoint is its anchor.
const cell = (channelId: string, scheduleBlockId: string, anchorMs: number): GuideFocusTarget => ({
  kind: "airing",
  selection: { anchorMs, channelId, scheduleBlockId } satisfies GuideSelection,
});

/**
 * The Guide the TV app runs: the real controller and focus registry, with guide-tv's cells
 * mounted as focus handles. Native focus landing on a cell reports it (guide-tv's onFocus); the
 * D-pad pressed past the timeline's edge lands on an edge stop, which calls `onTimeEdge`.
 */
const setup = async () => {
  const source = { load: vi.fn((window: typeof live) => Promise.resolve(windowGuide(window))) };
  const guide = createGuideController({ now: () => nowMs, resolveWindow: () => live, source });
  const registry = createTvGuideFocusRegistry();
  const focused = vi.fn<(key: string) => void>();
  const mount = (target: GuideFocusTarget, key: string) =>
    registry.register(target, { focus: () => focused(key) });
  const unmount = (target: GuideFocusTarget) => registry.register(target, null);
  const land = (target: GuideFocusTarget) => {
    registry.focused(target);
    if (target.kind === "airing") guide.select(target.selection);
  };
  const edge = (side: "left" | "right") => tvGuideTimeEdge(guide, registry, side, nowMs);
  await guide.refresh();
  return { edge, focused, guide, land, mount, registry, source, unmount };
};

describe("TV Guide time edges (app.tsx's onTimeEdge, #1659 decision N4)", () => {
  it("◀ off a programme already on now hands focus back without paging before now", async () => {
    const { edge, focused, land, mount, source } = await setup();
    const first = cell("split", "split-first-1000", 2_000);
    mount(first, "first");
    land(first);

    expect(await edge("left")).toBeUndefined();
    expect(source.load).toHaveBeenCalledTimes(1);
    expect(focused).toHaveBeenLastCalledWith("first");
  });

  it("◀ off a live-window programme that runs past now still never serves a window before now", async () => {
    const { edge, focused, guide, land, mount, source } = await setup();
    const spanning = cell("spanning", "spanning", 3_000);
    mount(spanning, "spanning");
    land(spanning);

    // The reducer asks for an earlier page; the controller clamps it to now and serves nothing new.
    expect(await edge("left")).toBeUndefined();
    expect(source.load).toHaveBeenCalledTimes(1);
    expect(guide.getSnapshot().layout).toMatchObject({ fromMs: live.from, toMs: live.to });
    expect(focused).toHaveBeenLastCalledWith("spanning");
  });

  it("◀ off a paged window's first programme leaves focus to the Guide's settled selection", async () => {
    const { edge, focused, guide, land, mount, registry, unmount } = await setup();
    await guide.page("later");
    const first = cell("split", "split-first-5000", 6_000);
    mount(first, "paged-first");
    land(first);
    // GuideJourney's selection effect, at its earliest: it asks for the settled selection as soon
    // as the controller publishes, before the edge handler resumes.
    guide.subscribe(() => {
      const { selection, status } = guide.getSnapshot();
      if (status === "ready" && selection) registry.request({ kind: "airing", selection });
    });

    const paging = edge("left");
    unmount(first);
    expect(await paging).toBe("earlier");
    const settled = guide.getSnapshot().selection!;
    mount({ kind: "airing", selection: settled }, "live-settled");
    expect(focused).toHaveBeenLastCalledWith("live-settled");
  });

  it("▶ off a whole-window programme pages forward and keeps focus on it; ◀ there pages back to now", async () => {
    const { edge, focused, guide, land, mount, source, unmount } = await setup();
    const spanning = cell("spanning", "spanning", 3_000);
    mount(spanning, "live-spanning");
    land(spanning);

    // The reload re-mounts the grid: the old cell goes before the paged one mounts.
    const paging = edge("right");
    unmount(spanning);
    expect(await paging).toBe("later");
    expect(source.load).toHaveBeenLastCalledWith({ from: 5_000, to: 9_000 }, expect.any(AbortSignal));
    expect(guide.getSnapshot().selection).toMatchObject({
      channelId: "spanning",
      scheduleBlockId: "spanning",
    });
    const paged = cell("spanning", "spanning", 7_000);
    mount(paged, "paged-spanning");
    expect(focused).toHaveBeenLastCalledWith("paged-spanning");

    land(paged);
    expect(await edge("left")).toBe("earlier");
    expect(source.load).toHaveBeenLastCalledWith(live, expect.any(AbortSignal));
    expect(guide.getSnapshot().layout).toMatchObject({ fromMs: live.from });
  });

  it("▶ off a trailing programme that doesn't span the window hands focus back", async () => {
    const { edge, focused, land, mount, source } = await setup();
    const second = cell("split", "split-second-1000", 4_000);
    mount(second, "second");
    land(second);

    expect(await edge("right")).toBeUndefined();
    expect(source.load).toHaveBeenCalledTimes(1);
    expect(focused).toHaveBeenLastCalledWith("second");
  });

  it("an edge reached from a filter hands focus back to that filter", async () => {
    const { edge, focused, land, mount, source } = await setup();
    const filter: GuideFocusTarget = { filter: "all", kind: "filter" };
    mount(filter, "all");
    land(filter);

    expect(await edge("left")).toBeUndefined();
    expect(source.load).toHaveBeenCalledTimes(1);
    expect(focused).toHaveBeenLastCalledWith("all");
  });
});
