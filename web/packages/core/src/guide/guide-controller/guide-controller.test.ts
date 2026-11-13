import type { GuideOutputBody } from "@loomarr/api/models/guideOutputBody";
import { describe, expect, it, vi } from "vitest";

import { createGuideController, createGuideSourcePort } from "./guide-controller";

const guide = (channelId = "seven"): GuideOutputBody => ({
  channels: [
    {
      airings: [
        {
          kind: "program",
          scheduleBlockId: `airing-${channelId}-one`,
          startMs: 1_000,
          stopMs: 4_000,
          title: "The Seventh Postcard",
        },
        {
          kind: "program",
          scheduleBlockId: `airing-${channelId}-two`,
          startMs: 4_000,
          stopMs: 5_000,
          title: "Comet Cadet",
        },
      ],
      channelId,
      name: "Science Fiction",
      number: 7,
      pendingCount: 0,
      status: "live",
    },
  ],
  fromMs: 1_000,
  toMs: 5_000,
});

const deferred = <T>() => {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => {
    resolve = done;
  });
  return { promise, resolve };
};

describe("Guide controller", () => {
  it("loads the resolved authoritative window and selects the preferred channel at now", async () => {
    const source = { load: vi.fn().mockResolvedValue(guide()) };
    const resolveWindow = vi.fn().mockReturnValue({ from: 1_000, to: 5_000 });
    const controller = createGuideController({ now: () => 2_000, resolveWindow, source });

    await controller.refresh("seven");

    expect(resolveWindow).toHaveBeenCalledWith(2_000);
    expect(source.load).toHaveBeenCalledWith({ from: 1_000, to: 5_000 }, expect.any(AbortSignal));
    expect(controller.getSnapshot()).toMatchObject({
      selection: { anchorMs: 2_000, channelId: "seven", scheduleBlockId: "airing-seven-one" },
      status: "ready",
    });
  });

  it("narrows to a filter's channels in guide order, and moves only among them", async () => {
    const lineup: GuideOutputBody = {
      ...guide(),
      channels: ["two", "four", "seven", "nine"].flatMap((id, index) =>
        guide(id).channels.map((channel) => ({ ...channel, number: index + 1 })),
      ),
    };
    const controller = createGuideController({
      now: () => 2_000,
      source: { load: vi.fn().mockResolvedValue(lineup) },
    });
    await controller.refresh("four");
    const rows = () => controller.getSnapshot().layout?.channels.map((channel) => channel.source.channelId);

    // Recent arrives newest first; the Guide still reads by number.
    controller.restrict(["nine", "two"]);
    expect(rows()).toEqual(["two", "nine"]);
    expect(controller.getSnapshot().selection).toMatchObject({ anchorMs: 2_000, channelId: "two" });
    expect(controller.move("down")?.selection.channelId).toBe("nine");
    expect(controller.move("down")?.boundary).toBe("down");

    controller.restrict(undefined);
    expect(rows()).toEqual(["two", "four", "seven", "nine"]);
    expect(controller.getSnapshot().selection?.channelId).toBe("nine");

    // A filter set before the guide loads applies to it.
    controller.restrict(["seven"]);
    await controller.refresh();
    expect(rows()).toEqual(["seven"]);
  });

  it("keeps only the latest refresh authoritative", async () => {
    const first = deferred<GuideOutputBody>();
    const second = deferred<GuideOutputBody>();
    const signals: AbortSignal[] = [];
    const source = {
      load: vi.fn((_window, signal: AbortSignal) => {
        signals.push(signal);
        return signals.length === 1 ? first.promise : second.promise;
      }),
    };
    const controller = createGuideController({ now: () => 2_000, source });
    const stale = controller.refresh();
    const latest = controller.refresh("latest");
    expect(signals[0]?.aborted).toBe(true);

    second.resolve(guide("latest"));
    await latest;
    first.resolve(guide("stale"));
    await stale;

    expect(controller.getSnapshot().selection?.channelId).toBe("latest");
  });

  it("preserves the selected time column across refreshes", async () => {
    const source = { load: vi.fn().mockResolvedValue(guide()) };
    const controller = createGuideController({ now: () => 2_000, source });
    await controller.refresh();
    controller.select({ anchorMs: 4_500, channelId: "seven", scheduleBlockId: "airing-seven-two" });

    await controller.refresh();

    expect(controller.getSnapshot().selection).toEqual({
      anchorMs: 4_500,
      channelId: "seven",
      scheduleBlockId: "airing-seven-two",
    });
  });

  it("exposes platform-neutral movement and its boundaries", async () => {
    const controller = createGuideController({
      now: () => 2_000,
      source: { load: vi.fn().mockResolvedValue(guide()) },
    });
    await controller.refresh();

    expect(controller.move("right")?.selection.scheduleBlockId).toBe("airing-seven-two");
    expect(controller.getSnapshot().selection?.scheduleBlockId).toBe("airing-seven-two");
    expect(controller.move("right")?.boundary).toBe("right");
  });

  it("pages the window earlier and later without crossing the live window (#1659 decision N4)", async () => {
    const resolveWindow = vi.fn().mockReturnValue({ from: 1_000, to: 5_000 });
    const source = {
      load: vi.fn((window: { from: number; to: number }) =>
        Promise.resolve({ ...guide(`w${window.from}`), fromMs: window.from, toMs: window.to }),
      ),
    };
    const controller = createGuideController({ now: () => 2_000, resolveWindow, source });
    await controller.refresh();
    expect(controller.getSnapshot().layout).toMatchObject({ fromMs: 1_000, toMs: 5_000 });

    // Already at the live window: LEFT has nowhere earlier to go.
    await controller.page("earlier");
    expect(source.load).toHaveBeenCalledTimes(1);
    expect(controller.getSnapshot().layout).toMatchObject({ fromMs: 1_000, toMs: 5_000 });

    await controller.page("later");
    expect(source.load).toHaveBeenLastCalledWith({ from: 5_000, to: 9_000 }, expect.any(AbortSignal));
    expect(controller.getSnapshot().layout).toMatchObject({ fromMs: 5_000, toMs: 9_000 });

    await controller.page("earlier");
    expect(source.load).toHaveBeenLastCalledWith({ from: 1_000, to: 5_000 }, expect.any(AbortSignal));
    expect(controller.getSnapshot().layout).toMatchObject({ fromMs: 1_000, toMs: 5_000 });
  });

  // A refresh re-reads what the viewer is looking at: a channel event, a reconnect or a tune must not
  // throw them back to now from a later page. Once now catches up with that page, it is live again.
  it("refreshes the page the viewer moved to, until now reaches it", async () => {
    let at = 2_000;
    const resolveWindow = vi.fn((now: number) => {
      const from = Math.floor(now / 4_000) * 4_000 + 1_000;
      return { from, to: from + 4_000 };
    });
    const source = {
      load: vi.fn((window: { from: number; to: number }) =>
        Promise.resolve({ ...guide(), fromMs: window.from, toMs: window.to }),
      ),
    };
    const controller = createGuideController({ now: () => at, resolveWindow, source });
    await controller.refresh();
    await controller.page("later");
    await controller.page("later");
    expect(controller.getSnapshot().layout).toMatchObject({ fromMs: 9_000, toMs: 13_000 });

    await controller.refresh("seven");
    expect(source.load).toHaveBeenLastCalledWith({ from: 9_000, to: 13_000 }, expect.any(AbortSignal));
    expect(controller.getSnapshot().layout).toMatchObject({ fromMs: 9_000, toMs: 13_000 });

    // Now has caught up with that page: the live window it resolves to is the one to read.
    at = 14_000;
    await controller.refresh();
    expect(source.load).toHaveBeenLastCalledWith({ from: 13_000, to: 17_000 }, expect.any(AbortSignal));
  });

  it("falls back deterministically and reports an empty authoritative Guide", async () => {
    const source = { load: vi.fn().mockResolvedValue(guide("available")) };
    const controller = createGuideController({ now: () => 2_000, source });
    await controller.refresh("removed");
    expect(controller.getSnapshot().selection?.channelId).toBe("available");

    source.load.mockResolvedValue({ ...guide(), channels: [] });
    await controller.refresh();
    expect(controller.getSnapshot()).toMatchObject({ status: "empty" });
    expect(controller.getSnapshot().selection).toBeUndefined();
  });

  describe("a refresh over a shown Guide", () => {
    const watch = (controller: ReturnType<typeof createGuideController>) => {
      const seen: string[] = [];
      controller.subscribe(() => {
        const { layout, refreshing, status } = controller.getSnapshot();
        seen.push(`${status}/${layout ? "layout" : "no-layout"}${refreshing ? "/refreshing" : ""}`);
      });
      return seen;
    };

    it("shows the first load as loading", async () => {
      const controller = createGuideController({
        now: () => 2_000,
        source: { load: vi.fn().mockResolvedValue(guide()) },
      });
      const seen = watch(controller);
      await controller.refresh();
      expect(seen).toEqual(["loading/no-layout", "ready/layout"]);
    });

    it("keeps the layout ready while it re-reads, and flags the refresh", async () => {
      const source = { load: vi.fn().mockResolvedValue(guide()) };
      const controller = createGuideController({ now: () => 2_000, source });
      await controller.refresh();
      const seen = watch(controller);

      await controller.refresh();

      expect(seen).toEqual(["ready/layout/refreshing", "ready/layout"]);
      expect(controller.getSnapshot().refreshing).toBeUndefined();
    });

    it("keeps the layout and reports the error when a background refresh fails", async () => {
      const source = { load: vi.fn().mockResolvedValue(guide()) };
      const controller = createGuideController({ now: () => 2_000, source });
      await controller.refresh();
      const shown = controller.getSnapshot().layout;
      const seen = watch(controller);

      source.load.mockRejectedValue(new Error("Couldn't load the Guide (503)."));
      await controller.refresh();

      expect(seen).toEqual(["ready/layout/refreshing", "ready/layout"]);
      expect(controller.getSnapshot()).toMatchObject({
        error: "Couldn't load the Guide (503).",
        layout: shown,
        status: "ready",
      });
      expect(controller.getSnapshot().refreshing).toBeUndefined();

      source.load.mockResolvedValue(guide());
      await controller.refresh();
      expect(controller.getSnapshot().error).toBeUndefined();
    });

    // A retry that cleared the error at once unmounted the notice that offered it, and the focus
    // inside it fell to the page. The error holds until the retry itself answers.
    describe("retrying a failed refresh", () => {
      const failedOver = async () => {
        const source = { load: vi.fn().mockResolvedValue(guide()) };
        const controller = createGuideController({ now: () => 2_000, source });
        await controller.refresh();
        source.load.mockRejectedValue(new Error("Couldn't load the Guide (503)."));
        await controller.refresh();
        let settleRetry: (outcome: "ok" | "fail") => void = () => undefined;
        source.load.mockImplementation(
          () =>
            new Promise((resolve, reject) => {
              settleRetry = (outcome) =>
                outcome === "ok" ? resolve(guide()) : reject(new Error("Still down."));
            }),
        );
        return {
          controller,
          retry: () => controller.refresh(),
          settle: (o: "ok" | "fail") => settleRetry(o),
        };
      };

      it("keeps the error and flags the retry while it is in flight", async () => {
        const { controller, retry, settle } = await failedOver();

        const pending = retry();

        expect(controller.getSnapshot()).toMatchObject({
          error: "Couldn't load the Guide (503).",
          refreshing: true,
          status: "ready",
        });
        settle("ok");
        await pending;
      });

      it("clears the error and the flag when the retry works", async () => {
        const { controller, retry, settle } = await failedOver();
        const pending = retry();

        settle("ok");
        await pending;

        expect(controller.getSnapshot().error).toBeUndefined();
        expect(controller.getSnapshot().refreshing).toBeUndefined();
      });

      it("says the new error, and drops the flag, when the retry fails too", async () => {
        const { controller, retry, settle } = await failedOver();
        const pending = retry();

        settle("fail");
        await pending;

        expect(controller.getSnapshot()).toMatchObject({ error: "Still down.", status: "ready" });
        expect(controller.getSnapshot().refreshing).toBeUndefined();
      });

      it("keeps the error across a paged retry as well", async () => {
        const { controller, settle } = await failedOver();

        const pending = controller.page("later");

        expect(controller.getSnapshot()).toMatchObject({
          error: "Couldn't load the Guide (503).",
          refreshing: true,
        });
        settle("ok");
        await pending;
        expect(controller.getSnapshot().error).toBeUndefined();
      });

      it("reads the very first load as loading, with no error", () => {
        const source = { load: vi.fn().mockReturnValue(new Promise(() => undefined)) };
        const controller = createGuideController({ now: () => 2_000, source });

        void controller.refresh();

        expect(controller.getSnapshot()).toMatchObject({ status: "loading" });
        expect(controller.getSnapshot().error).toBeUndefined();
        expect(controller.getSnapshot().refreshing).toBeUndefined();
      });

      // The error panel's own Try again is what the viewer pressed; reading "loading" at once took
      // the panel, and the focus on its button, away.
      describe("a first load that failed", () => {
        const failedFirst = async () => {
          const source = { load: vi.fn().mockRejectedValue(new Error("offline")) };
          const controller = createGuideController({ now: () => 2_000, source });
          await controller.refresh();
          let settleRetry: (outcome: "ok" | "fail") => void = () => undefined;
          source.load.mockImplementation(
            () =>
              new Promise((resolve, reject) => {
                settleRetry = (outcome) =>
                  outcome === "ok" ? resolve(guide()) : reject(new Error("still offline"));
              }),
          );
          return { controller, settle: (o: "ok" | "fail") => settleRetry(o) };
        };

        it("stays an error, flagged as retrying, while its retry is in flight", async () => {
          const { controller, settle } = await failedFirst();

          const pending = controller.refresh();

          expect(controller.getSnapshot()).toMatchObject({
            error: "offline",
            refreshing: true,
            status: "error",
          });
          settle("ok");
          await pending;
        });

        it("becomes the Guide when the retry works", async () => {
          const { controller, settle } = await failedFirst();
          const pending = controller.refresh();

          settle("ok");
          await pending;

          expect(controller.getSnapshot()).toMatchObject({ status: "ready" });
          expect(controller.getSnapshot().error).toBeUndefined();
          expect(controller.getSnapshot().refreshing).toBeUndefined();
        });

        it("says the new error, with the flag dropped, when the retry fails too", async () => {
          const { controller, settle } = await failedFirst();
          const pending = controller.refresh();

          settle("fail");
          await pending;

          expect(controller.getSnapshot()).toMatchObject({ error: "still offline", status: "error" });
          expect(controller.getSnapshot().refreshing).toBeUndefined();
        });
      });
    });

    it("keeps a failed refresh's error, and a re-read in flight, across a filter change", async () => {
      const lineup: GuideOutputBody = {
        ...guide(),
        channels: ["two", "seven"].flatMap((id, index) =>
          guide(id).channels.map((channel) => ({ ...channel, number: index + 1 })),
        ),
      };
      const source = { load: vi.fn().mockResolvedValue(lineup) };
      const controller = createGuideController({ now: () => 2_000, source });
      await controller.refresh();

      source.load.mockRejectedValue(new Error("Couldn't load the Guide (503)."));
      await controller.refresh();
      // Choosing Favorites narrows what is on screen; the Guide on screen is still the stale one.
      controller.restrict(["seven"]);
      expect(controller.getSnapshot()).toMatchObject({
        error: "Couldn't load the Guide (503).",
        status: "ready",
      });

      const slow = deferred<GuideOutputBody>();
      source.load.mockReturnValue(slow.promise);
      const refreshed = controller.refresh();
      controller.restrict(undefined);
      expect(controller.getSnapshot().refreshing).toBe(true);
      slow.resolve(lineup);
      await refreshed;
      expect(controller.getSnapshot().refreshing).toBeUndefined();
      expect(controller.getSnapshot().error).toBeUndefined();
    });

    it("keeps an empty Guide empty while it re-reads", async () => {
      const source = { load: vi.fn().mockResolvedValue({ ...guide(), channels: [] }) };
      const controller = createGuideController({ now: () => 2_000, source });
      await controller.refresh();
      const seen = watch(controller);
      await controller.refresh();
      expect(seen).toEqual(["empty/layout/refreshing", "empty/layout"]);
    });

    it("keeps the layout while it pages", async () => {
      const source = { load: vi.fn().mockResolvedValue(guide()) };
      const controller = createGuideController({ now: () => 2_000, source });
      await controller.refresh();
      const seen = watch(controller);
      await controller.page("later");
      expect(seen).toEqual(["ready/layout/refreshing", "ready/layout"]);
    });
  });

  it("uses the generated Guide URL and reports HTTP failure without inventing data", async () => {
    const request = vi.fn().mockResolvedValue(new Response("no", { status: 503 }));
    const source = createGuideSourcePort(request);

    await expect(source.load({ from: 1_000, to: 5_000 }, new AbortController().signal)).rejects.toThrow(
      "Couldn't load the Guide (503).",
    );
    expect(request).toHaveBeenCalledWith(
      "/v1/guide?from=1000&to=5000",
      expect.objectContaining({ method: "GET" }),
    );
  });

  it("aborts work and stops publishing after disposal", async () => {
    const pending = deferred<GuideOutputBody>();
    let signal: AbortSignal | undefined;
    const controller = createGuideController({
      source: {
        load: vi.fn((_window, nextSignal: AbortSignal) => {
          signal = nextSignal;
          return pending.promise;
        }),
      },
    });
    const refresh = controller.refresh();
    controller.dispose();
    expect(signal?.aborted).toBe(true);
    pending.resolve(guide());
    await refresh;
    expect(controller.getSnapshot().status).toBe("loading");
  });
});
