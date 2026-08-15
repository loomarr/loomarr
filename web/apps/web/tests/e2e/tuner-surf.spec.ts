import type { Page } from "@playwright/test";
import { expect, test } from "@playwright/test";
import { channelId, installTunerBackend } from "./tuner-backend";

test.setTimeout(120_000);

const p95 = (samples: number[]): number => {
  const ordered = [...samples].sort((a, b) => a - b);
  return ordered[Math.max(0, Math.ceil(ordered.length * 0.95) - 1)] ?? Number.POSITIVE_INFINITY;
};

const installFrameClock = async (page: Page) => {
  await page.addInitScript(() => {
    // The 100-Channel scenario intentionally creates hundreds of API/HLS resource entries. Keep
    // Chromium's default 250-entry buffer from truncating the manifest timing evidence.
    performance.setResourceTimingBufferSize?.(2_000);
    const frames: Array<{ at: number; src: string }> = [];
    Object.defineProperty(window, "__loomarrDecodedFrames", { value: frames, configurable: true });
    const original = HTMLVideoElement.prototype.requestVideoFrameCallback;
    if (!original) return;
    Object.defineProperty(HTMLVideoElement.prototype, "requestVideoFrameCallback", {
      configurable: true,
      value(this: HTMLVideoElement, callback: VideoFrameRequestCallback) {
        return original.call(this, (now, metadata) => {
          frames.push({ at: performance.now(), src: this.currentSrc });
          callback(now, metadata);
        });
      },
    });
  });
};

const waitForDecodedFrame = async (page: Page) => {
  const play = page.getByRole("button", { name: "Play" });
  const decoded = page.waitForFunction(
    () =>
      ((window as Window & { __loomarrDecodedFrames?: number[] }).__loomarrDecodedFrames?.length ?? 0) > 0,
    undefined,
    { timeout: 10_000 },
  );

  // Firefox can reject autoplay after navigation, but the Play control is also visible briefly
  // BEFORE hls.js attaches MediaSource. Clicking in that empty-source window spends the synthetic
  // user gesture on a play() that cannot start, then leaves the real source paused. Race a normal
  // decoded frame against one fallback click made only after the video has an attached source.
  const fallbackPlay = Promise.all([
    play.waitFor({ state: "visible", timeout: 9_000 }),
    page.waitForFunction(() => Boolean(document.querySelector("video")?.currentSrc), undefined, {
      timeout: 9_000,
    }),
  ]).then(() => play.click());
  await Promise.race([decoded, fallbackPlay]);
  await decoded;
};

// Change routes through the already-running application. `page.goto` tears down the document,
// recompiles the app bundle, and cold-starts a new decoder on every sample; that measures browser
// startup rather than a viewer selecting another Channel. The real-runtime gate owns cold boot.
// A popstate navigation is the platform-neutral route input behind browser Back/Forward and lands
// in the same TanStack Router path as an in-app Link without coupling this transport test to the
// Guide's virtualized row layout.
const tuneInApp = async (page: Page, id: string): Promise<number> => {
  const before = await page.locator("video").evaluate((video) => ({
    src: video.currentSrc,
    count: (
      window as Window & {
        __loomarrDecodedFrames: Array<{ at: number; src: string }>;
      }
    ).__loomarrDecodedFrames.length,
  }));
  const started = await page.evaluate(() => performance.now());
  await page.evaluate((path) => {
    window.history.pushState({}, "", path);
    window.dispatchEvent(new PopStateEvent("popstate", { state: window.history.state }));
  }, `/channels/${id}/watch`);
  await expect(page).toHaveURL(new RegExp(`/channels/${id}/watch$`));
  await page.waitForFunction(
    ({ count, src }) =>
      (
        window as Window & {
          __loomarrDecodedFrames?: Array<{ at: number; src: string }>;
        }
      ).__loomarrDecodedFrames
        ?.slice(count)
        .some((frame) => frame.src !== src) ?? false,
    before,
    { timeout: 10_000 },
  );
  const decodedAt = await page.evaluate(
    ({ count, src }) =>
      (
        window as Window & {
          __loomarrDecodedFrames: Array<{ at: number; src: string }>;
        }
      ).__loomarrDecodedFrames
        .slice(count)
        .find((frame) => frame.src !== src)?.at ?? Number.POSITIVE_INFINITY,
    before,
  );
  return decodedAt - started;
};

const adjacentNumbers = (number: number): [number, number] => [
  number === 1 ? 100 : number - 1,
  number === 100 ? 1 : number + 1,
];

const waitForAdjacentWarm = async (
  backend: Awaited<ReturnType<typeof installTunerBackend>>,
  number: number,
  after: ReadonlyMap<string, number> = new Map(),
) => {
  for (const neighbor of adjacentNumbers(number)) {
    const asset = `${channelId(neighbor)}/segment.m4s`;
    await expect
      .poll(() => backend.state.assetRequests.filter((candidate) => candidate === asset).length)
      .toBeGreaterThan(after.get(asset) ?? 0);
  }
};

const adjacentWarmCounts = (
  backend: Awaited<ReturnType<typeof installTunerBackend>>,
  number: number,
): ReadonlyMap<string, number> =>
  new Map(
    adjacentNumbers(number).map((neighbor) => {
      const asset = `${channelId(neighbor)}/segment.m4s`;
      return [asset, backend.state.assetRequests.filter((candidate) => candidate === asset).length];
    }),
  );

test("100-channel tuner meets surf latency and latest-request-wins gates", async ({ page }) => {
  await installFrameClock(page);
  const backend = await installTunerBackend(page);

  // Prove the engine can cold-start and decode the representative H.264 stream, but keep decoder
  // bootstrap out of the surf percentile. The real-runtime gate owns cold boot timing; this gate
  // owns an already-running tuner and never retries a black start.
  await page.goto(`/channels/${channelId(1)}/watch`);
  await waitForDecodedFrame(page);
  await expect(page.locator("video")).toHaveCount(1);
  await waitForAdjacentWarm(backend, 1);

  // Arbitrary prepared tune: navigate the already-running app to a non-adjacent Channel. Measure
  // route request to a genuinely decoded frame, including SPA/API/HLS work but excluding the cold
  // document + decoder bootstrap proven above and owned by the real-runtime gate.
  const arbitrary: number[] = [];
  for (const number of [3, 17, 29, 41, 53, 67, 79, 91, 8, 50, 62, 74, 86, 98, 12, 24, 36, 48, 60, 72]) {
    arbitrary.push(await tuneInApp(page, channelId(number)));
    await expect(page.locator("video")).toHaveCount(1);
    // Keep arbitrary samples independent. Rapid sequential intent has its own adjacent and burst
    // gates below; this percentile starts after the previous Channel's bounded hot set has settled.
    await waitForAdjacentWarm(backend, number);
  }
  expect(
    p95(arbitrary),
    `arbitrary prepared p95: ${p95(arbitrary).toFixed(1)}ms; samples: ${arbitrary
      .map((sample) => sample.toFixed(1))
      .join(", ")}`,
  ).toBeLessThan(1_500);

  // Start the adjacent run from the middle of the catalog and prove speculative work is prepared-only.
  const probeStart = backend.state.preparedProbes.length;
  await page.goto(`/channels/${channelId(50)}/watch`);
  await waitForDecodedFrame(page);
  await expect
    .poll(() => backend.state.preparedProbes.slice(probeStart))
    .toEqual(expect.arrayContaining([channelId(49), channelId(51)]));
  expect(backend.state.activeManifests.slice(-1)).toEqual([channelId(50)]);

  // A slow replacement must leave the last decoded picture behind the target OSD. This is one
  // video element and one decoder: the outgoing frame is held as a poster while transport swaps.
  const video = page.locator("video");
  await backend.delayNextActiveManifest(channelId(51), 500);
  const heldFrameCount = await page.evaluate(
    () => performance.getEntriesByName("loomarr:tune:request-to-first-frame").length,
  );
  await page.getByRole("button", { name: "Channel up" }).click();
  await expect(page.getByRole("status")).toContainText("CH 51");
  await expect(video).toHaveAttribute("poster", /^data:image\/png;base64,/);
  await page.waitForFunction(
    (count) => performance.getEntriesByName("loomarr:tune:request-to-first-frame").length > count,
    heldFrameCount,
  );
  await expect(video).not.toHaveAttribute("poster", /^data:image\/png;base64,/);

  // Reset to the middle so the measured adjacent run remains the same 50 → 70 sample.
  const resetWarmCounts = adjacentWarmCounts(backend, 50);
  await page.goto(`/channels/${channelId(50)}/watch`);
  await waitForDecodedFrame(page);
  await waitForAdjacentWarm(backend, 50, resetWarmCounts);

  await page.evaluate(() => {
    performance.clearMeasures("loomarr:tune:request-to-osd");
    performance.clearMeasures("loomarr:tune:request-to-first-frame");
  });
  const osd: number[] = [];
  const adjacentFrames: number[] = [];
  let current = 50;
  for (let sample = 0; sample < 20; sample++) {
    const target = current === 100 ? 1 : current + 1;
    const id = channelId(target);
    const targetMints = backend.state.playURLMints.filter((candidate) => candidate === id).length;
    const next = target === 100 ? 1 : target + 1;
    const nextAsset = `${channelId(next)}/segment.m4s`;
    const nextAssetRequests = backend.state.assetRequests.filter(
      (candidate) => candidate === nextAsset,
    ).length;

    const previousFrames = await page.evaluate(
      () => performance.getEntriesByName("loomarr:tune:request-to-first-frame").length,
    );
    await page.getByRole("button", { name: "Channel up" }).click();
    await expect(page).toHaveURL(new RegExp(`/channels/${id}/watch$`));
    await page.waitForFunction(
      (count) => performance.getEntriesByName("loomarr:tune:request-to-first-frame").length > count,
      previousFrames,
    );

    const timing = await page.evaluate(() => {
      const latest = (name: string) =>
        performance.getEntriesByName(name).at(-1) as PerformanceMeasure | undefined;
      const osd = latest("loomarr:tune:request-to-osd");
      const frame = latest("loomarr:tune:request-to-first-frame");
      return {
        osd: osd?.duration ?? Number.POSITIVE_INFINITY,
        frame: frame?.duration ?? Number.POSITIVE_INFINITY,
        detail: frame?.detail as { adjacent?: boolean; warmed?: boolean } | undefined,
      };
    });
    expect(timing.detail).toMatchObject({ adjacent: true, warmed: true });
    osd.push(timing.osd);
    adjacentFrames.push(timing.frame);
    expect(backend.state.playURLMints.filter((candidate) => candidate === id)).toHaveLength(targetMints);
    await expect(page.locator("video")).toHaveCount(1);
    // The frame makes this Channel active; its post-frame warmer must finish the next Channel
    // before the following measured click. Count from this iteration so an identical request made
    // during the arbitrary phase cannot masquerade as the current controller's warm.
    await expect
      .poll(() => backend.state.assetRequests.filter((candidate) => candidate === nextAsset).length)
      .toBeGreaterThan(nextAssetRequests);
    current = target;
  }

  expect(p95(osd), `OSD p95: ${p95(osd).toFixed(1)}ms`).toBeLessThan(100);
  expect(
    p95(adjacentFrames),
    `prepared adjacent first-frame p95: ${p95(adjacentFrames).toFixed(1)}ms`,
  ).toBeLessThan(750);

  const manifestDurations = await page.evaluate(() =>
    performance
      .getEntriesByType("resource")
      .filter((entry) => entry.name.includes("/master.m3u8") && entry.name.includes("mode=prepared"))
      .map((entry) => entry.duration),
  );
  expect(manifestDurations.length).toBeGreaterThanOrEqual(20);
  expect(
    p95(manifestDurations),
    `prepared manifest p95: ${p95(manifestDurations).toFixed(1)}ms`,
  ).toBeLessThan(50);

  // Twenty mixed requests in one burst must collapse to the last intent. The first click provides
  // real user activation; the remaining clicks happen in one task, before route churn can sequence
  // them, which is the failure mode pendingId/latest-request-wins exists to prevent.
  const directions = [1, -1, 1, 1, -1, 1, -1, -1, 1, 1, 1, -1, 1, -1, 1, 1, -1, 1, -1, 1] as const;
  const expected = directions.reduce(
    (number, direction) => ((number - 1 + direction + 100) % 100) + 1,
    current,
  );
  const beforeBurstFrames = await page.evaluate(
    () => performance.getEntriesByName("loomarr:tune:request-to-first-frame").length,
  );
  await page.getByRole("button", { name: "Channel up" }).click();
  await page.evaluate((remaining) => {
    for (const direction of remaining) {
      const label = direction > 0 ? "Channel up" : "Channel down";
      (document.querySelector(`[aria-label="${label}"]`) as HTMLButtonElement | null)?.click();
    }
  }, directions.slice(1));

  const finalId = channelId(expected);
  await expect(page).toHaveURL(new RegExp(`/channels/${finalId}/watch$`));
  await page.waitForFunction(
    (count) => performance.getEntriesByName("loomarr:tune:request-to-first-frame").length > count,
    beforeBurstFrames,
  );
  const burst = await page.evaluate((count) => {
    const frames = performance.getEntriesByName(
      "loomarr:tune:request-to-first-frame",
    ) as PerformanceMeasure[];
    const requestIds = performance
      .getEntriesByType("mark")
      .map((entry) => entry.name.match(/^loomarr:tune:(\d+):request$/)?.[1])
      .filter((id): id is string => Boolean(id))
      .map(Number);
    return {
      addedFrames: frames.length - count,
      finalAttempt: (frames.at(-1)?.detail as { attemptId?: number } | undefined)?.attemptId,
      latestAttempt: Math.max(...requestIds),
    };
  }, beforeBurstFrames);
  expect(burst).toMatchObject({ addedFrames: 1, finalAttempt: burst.latestAttempt });
  await expect(page.locator("video")).toHaveCount(1);
  await expect(page.getByRole("heading", { name: `Watch Channel ${expected}` })).toBeVisible();
  expect(backend.state.activeManifests.at(-1)).toBe(finalId);
});
