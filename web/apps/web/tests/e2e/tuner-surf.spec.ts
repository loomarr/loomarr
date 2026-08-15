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
    const frames: number[] = [];
    Object.defineProperty(window, "__loomarrDecodedFrames", { value: frames, configurable: true });
    const original = HTMLVideoElement.prototype.requestVideoFrameCallback;
    if (!original) return;
    Object.defineProperty(HTMLVideoElement.prototype, "requestVideoFrameCallback", {
      configurable: true,
      value(this: HTMLVideoElement, callback: VideoFrameRequestCallback) {
        return original.call(this, (now, metadata) => {
          frames.push(performance.now());
          callback(now, metadata);
        });
      },
    });
  });
};

const waitForDecodedFrame = async (page: Page) => {
  const play = page.getByRole("button", { name: "Play" });
  if (await play.isVisible()) await play.click();
  await page.waitForFunction(
    () =>
      ((window as Window & { __loomarrDecodedFrames?: number[] }).__loomarrDecodedFrames?.length ?? 0) > 0,
  );
};

test("100-channel tuner meets surf latency and latest-request-wins gates", async ({ page }) => {
  await installFrameClock(page);
  const backend = await installTunerBackend(page);

  // Arbitrary prepared tune: a deep link is the platform-neutral form of selecting a non-adjacent
  // channel. Measure navigation request to a genuinely decoded frame, including SPA/API/HLS work.
  const arbitrary: number[] = [];
  for (const number of [3, 17, 29, 41, 53, 67, 79, 91, 8, 50]) {
    await page.goto(`/channels/${channelId(number)}/watch`);
    await waitForDecodedFrame(page);
    arbitrary.push(
      await page.evaluate(
        () =>
          (window as Window & { __loomarrDecodedFrames: number[] }).__loomarrDecodedFrames.at(0) ??
          Number.POSITIVE_INFINITY,
      ),
    );
    await expect(page.locator("video")).toHaveCount(1);
  }
  expect(p95(arbitrary), `arbitrary prepared p95: ${p95(arbitrary).toFixed(1)}ms`).toBeLessThan(1_500);

  // Start the adjacent run from the middle of the catalog and prove speculative work is prepared-only.
  const probeStart = backend.state.preparedProbes.length;
  await page.goto(`/channels/${channelId(50)}/watch`);
  await waitForDecodedFrame(page);
  await expect
    .poll(() => backend.state.preparedProbes.slice(probeStart))
    .toEqual(expect.arrayContaining([channelId(49), channelId(51)]));
  expect(backend.state.activeManifests.slice(-1)).toEqual([channelId(50)]);

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
    await expect
      .poll(() => backend.state.assetRequests.filter((asset) => asset === `${id}/segment.m4s`).length)
      .toBeGreaterThan(0);

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
