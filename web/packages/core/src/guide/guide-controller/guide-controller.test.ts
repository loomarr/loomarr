import type { GuideOutputBody } from "@loomarr/api/models/guideOutputBody";
import { describe, expect, it, vi } from "vitest";
import { createGuideController, createGuideSourcePort } from "./guide-controller";

const guide = (channelId = "seven"): GuideOutputBody => ({
  channels: [
    {
      airings: [
        {
          kind: "program",
          scheduleBlockId: "airing-seven",
          startMs: 1_000,
          stopMs: 4_000,
          title: "Radioactive Man",
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
  it("loads the authoritative window and selects the requested channel at the current time", async () => {
    const source = { load: vi.fn().mockResolvedValue(guide()) };
    const controller = createGuideController({ now: () => 2_000, source });

    await controller.refresh("seven");

    expect(source.load).toHaveBeenCalledWith(
      { from: expect.any(Number), to: expect.any(Number) },
      expect.anything(),
    );
    expect(controller.getSnapshot()).toMatchObject({
      selection: { anchorMs: 2_000, channelId: "seven", scheduleBlockId: "airing-seven" },
      status: "ready",
    });
  });

  it("keeps the latest refresh authoritative and preserves the selected time column", async () => {
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

    expect(controller.getSnapshot()).toMatchObject({
      selection: { channelId: "latest" },
      status: "ready",
    });
  });

  it("falls back deterministically when a preferred channel leaves the served Guide", async () => {
    const controller = createGuideController({
      now: () => 2_000,
      source: { load: vi.fn().mockResolvedValue(guide("available")) },
    });

    await controller.refresh("removed");

    expect(controller.getSnapshot()).toMatchObject({
      selection: { channelId: "available" },
      status: "ready",
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
});
