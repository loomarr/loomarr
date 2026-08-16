import type { Page } from "@playwright/test";
import { expect, test } from "@playwright/test";
import { adjacentWarmMarkName } from "../../src/channels/tuner-timing";
import { channelId, installTunerBackend, tunerManifest } from "./tuner-backend";

test.setTimeout(120_000);

const TARGET_PLAYING_TIMEOUT_MS = 250;

const p95 = (samples: number[]): number => {
  const ordered = [...samples].sort((a, b) => a - b);
  return ordered[Math.max(0, Math.ceil(ordered.length * 0.95) - 1)] ?? Number.POSITIVE_INFINITY;
};

const installFrameClock = async (page: Page) => {
  await page.addInitScript(() => {
    // The 100-Channel scenario intentionally creates hundreds of API/HLS resource entries. Keep
    // Chromium's default 250-entry buffer from truncating the manifest timing evidence.
    performance.setResourceTimingBufferSize?.(2_000);
    const frames: Array<{ at: number; channel: string; ended: boolean; paused: boolean; src: string }> = [];
    Object.defineProperty(window, "__loomarrDecodedFrames", { value: frames, configurable: true });
    const mediaEvents: Array<{
      at: number;
      channel: string;
      currentTime: number;
      readyState: number;
      type: string;
    }> = [];
    Object.defineProperty(window, "__loomarrMediaEvents", { value: mediaEvents, configurable: true });
    for (const type of ["loadstart", "loadedmetadata", "loadeddata", "canplay", "play", "playing"]) {
      document.addEventListener(
        type,
        (event) => {
          if (!(event.target instanceof HTMLVideoElement)) return;
          mediaEvents.push({
            at: performance.now(),
            channel: event.target.dataset.playbackChannel ?? "",
            currentTime: event.target.currentTime,
            readyState: event.target.readyState,
            type,
          });
        },
        true,
      );
    }
    const original = HTMLVideoElement.prototype.requestVideoFrameCallback;
    if (!original) return;
    Object.defineProperty(HTMLVideoElement.prototype, "requestVideoFrameCallback", {
      configurable: true,
      value(this: HTMLVideoElement, callback: VideoFrameRequestCallback) {
        const channel = this.dataset.playbackChannel ?? "";
        return original.call(this, (now, metadata) => {
          frames.push({
            at: performance.now(),
            channel,
            ended: this.ended,
            paused: this.paused,
            src: this.currentSrc,
          });
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
  await page.waitForFunction(() => Boolean(document.querySelector("video")?.currentSrc), undefined, {
    timeout: 9_000,
  });
  // Give permitted autoplay a brief opportunity. If it produced no frame, activate the actual
  // Play control with keyboard/remote semantics; this avoids spending the gesture before a source
  // exists and avoids racing the auto-hidden pointer layer.
  await Promise.race([decoded, page.waitForTimeout(500)]);
  const frameCount = await page.evaluate(
    () => (window as Window & { __loomarrDecodedFrames?: number[] }).__loomarrDecodedFrames?.length ?? 0,
  );
  if (frameCount === 0 && (await play.count()) > 0) {
    await play.focus();
    await page.keyboard.press("Enter");
  }
  await decoded;
};

const waitForTargetPlaying = async (page: Page, channel: string, since: number) => {
  try {
    await page.waitForFunction(
      ({ channel, since }) =>
        (
          window as Window & {
            __loomarrMediaEvents?: Array<{ at: number; channel: string; type: string }>;
          }
        ).__loomarrMediaEvents?.some(
          (event) => event.at >= since && event.channel === channel && event.type === "playing",
        ) ?? false,
      { channel, since },
      { timeout: TARGET_PLAYING_TIMEOUT_MS },
    );
  } catch (error) {
    const playback = await playbackSnapshot(page, channel, since);
    throw new Error(
      `${channel} did not join target playback within ${TARGET_PLAYING_TIMEOUT_MS}ms: ${JSON.stringify(playback)}`,
      { cause: error },
    );
  }

  const playback = await playbackSnapshot(page, channel, since);
  const firstFrame = playback.frames?.at(0);
  const playing = playback.events?.find((event) => event.type === "playing");
  expect(
    (playing?.at ?? Number.POSITIVE_INFINITY) - (firstFrame?.at ?? Number.NEGATIVE_INFINITY),
    `${channel} target playback join lag: ${JSON.stringify(playback)}`,
  ).toBeLessThanOrEqual(TARGET_PLAYING_TIMEOUT_MS);
  // WebKit may deliver the first attributed rVFC just before its target-generation playing event.
  // The decoded-frame timestamp still owns latency; this bounded join proves the replacement did
  // not remain paused. A compact VOD may naturally end after that playing proof.
  expect(playback.paused && !playback.ended, `${channel} playback state: ${JSON.stringify(playback)}`).toBe(
    false,
  );
};

// Change routes through the already-running application. `page.goto` tears down the document,
// recompiles the app bundle, and cold-starts a new decoder on every sample; that measures browser
// startup rather than a viewer selecting another Channel. The real-runtime gate owns cold boot.
// A popstate navigation is the platform-neutral route input behind browser Back/Forward and lands
// in the same TanStack Router path as an in-app Link without coupling this transport test to the
// Guide's virtualized row layout. Dispatch it from a trusted, otherwise-unused key event so WebKit
// sees the same user activation a remote/keyboard channel choice supplies; page.evaluate alone is
// not a viewer action and is therefore forbidden from starting the replacement media.
const tuneInApp = async (page: Page, id: string): Promise<{ duration: number; trace: string }> => {
  const before = await page.locator("video").evaluate(() => ({
    count: (
      window as Window & {
        __loomarrDecodedFrames: Array<{
          at: number;
          channel: string;
          ended: boolean;
          paused: boolean;
          src: string;
        }>;
      }
    ).__loomarrDecodedFrames.length,
  }));
  const started = await page.evaluate(() => performance.now());
  await page.evaluate((path) => {
    window.addEventListener(
      "keydown",
      () => {
        window.history.pushState({}, "", path);
        window.dispatchEvent(new PopStateEvent("popstate", { state: window.history.state }));
      },
      { once: true },
    );
  }, `/channels/${id}/watch`);
  await page.keyboard.press("x");
  await expect(page).toHaveURL(new RegExp(`/channels/${id}/watch$`));
  try {
    await page.waitForFunction(
      ({ channel, count }) =>
        (
          window as Window & {
            __loomarrDecodedFrames?: Array<{
              at: number;
              channel: string;
              ended: boolean;
              paused: boolean;
              src: string;
            }>;
          }
        ).__loomarrDecodedFrames
          ?.slice(count)
          .some((frame) => frame.channel === channel) ?? false,
      { channel: id, count: before.count },
      { timeout: 10_000 },
    );
  } catch (error) {
    const playback = await playbackSnapshot(page, id, started);
    throw new Error(`${id} produced no target frame: ${JSON.stringify(playback)}`, { cause: error });
  }
  const decoded = await page.evaluate(
    ({ channel, count }) =>
      (
        window as Window & {
          __loomarrDecodedFrames: Array<{
            at: number;
            channel: string;
            ended: boolean;
            paused: boolean;
            src: string;
          }>;
        }
      ).__loomarrDecodedFrames
        .slice(count)
        .find((frame) => frame.channel === channel),
    { channel: id, count: before.count },
  );
  await waitForTargetPlaying(page, id, started);
  const trace = await page.evaluate((since) => {
    const resources = performance
      .getEntriesByType("resource")
      .filter((entry) => entry.startTime >= since)
      .filter((entry) => /play-url|master\.m3u8|init\.mp4|segment\.m4s/.test(entry.name))
      .map(
        (entry) =>
          `${new URL(entry.name).pathname.split("/").at(-1)}@${(entry.responseEnd - since).toFixed(0)}`,
      );
    const events = (
      window as Window & {
        __loomarrMediaEvents?: Array<{ at: number; readyState: number; type: string }>;
      }
    ).__loomarrMediaEvents
      ?.filter((event) => event.at >= since)
      .map((event) => `${event.type}${event.readyState}@${(event.at - since).toFixed(0)}`);
    return [...resources, ...(events ?? [])].join(" ");
  }, started);
  return { duration: (decoded?.at ?? Number.POSITIVE_INFINITY) - started, trace };
};

const adjacentNumbers = (number: number): [number, number] => [
  number === 1 ? 100 : number - 1,
  number === 100 ? 1 : number + 1,
];

const latestWarmAt = (page: Page, number: number) =>
  page.evaluate(
    (name) => {
      const mark = performance.getEntriesByName(name, "mark").at(-1);
      return mark ? performance.timeOrigin + mark.startTime : -1;
    },
    adjacentWarmMarkName(channelId(number)),
  );

const channelUp = async (page: Page) => {
  // Keep the real focusable control as the input seam, but activate it the way a keyboard/remote
  // does. Pointer activation can race the player's auto-hidden control bar in slow WebKit; focus
  // holds the chrome open and Enter is the same button action a TV remote will deliver.
  const button = page.getByRole("button", { name: "Channel up" });
  await button.focus();
  await page.keyboard.press("Enter");
};

const waitForAdjacentWarm = async (
  page: Page,
  number: number,
  after: ReadonlyMap<number, number> = new Map(),
) => {
  for (const neighbor of adjacentNumbers(number)) {
    await expect.poll(() => latestWarmAt(page, neighbor)).toBeGreaterThan(after.get(neighbor) ?? -1);
  }
};

const adjacentWarmCounts = (page: Page, number: number): Promise<ReadonlyMap<number, number>> =>
  Promise.all(
    adjacentNumbers(number).map(async (neighbor) => [neighbor, await latestWarmAt(page, neighbor)] as const),
  ).then((entries) => new Map(entries));

const playbackSnapshot = async (page: Page, channel: string, since: number) =>
  page.locator("video").evaluate(
    (element, { channel, since }) => ({
      currentTime: element.currentTime,
      duration: element.duration,
      ended: element.ended,
      events: (
        window as Window & {
          __loomarrMediaEvents?: Array<{
            at: number;
            channel: string;
            currentTime: number;
            readyState: number;
            type: string;
          }>;
        }
      ).__loomarrMediaEvents?.filter((event) => event.at >= since && event.channel === channel),
      frames: (
        window as Window & {
          __loomarrDecodedFrames?: Array<{
            at: number;
            channel: string;
            ended: boolean;
            paused: boolean;
            src: string;
          }>;
        }
      ).__loomarrDecodedFrames?.filter((frame) => frame.at >= since && frame.channel === channel),
      paused: element.paused,
      readyState: element.readyState,
      marks: performance
        .getEntriesByType("mark")
        .filter((entry) => entry.startTime >= since && entry.name.startsWith("loomarr:tune:"))
        .map((entry) => ({ name: entry.name, at: entry.startTime - since })),
      resources: performance
        .getEntriesByType("resource")
        .filter((entry) => entry.startTime >= since)
        .filter((entry) => /play-url|master\.m3u8|init\.mp4|segment\.m4s/.test(entry.name))
        .map((entry) => ({
          name: new URL(entry.name).pathname,
          responseEnd: entry.responseEnd - since,
          startTime: entry.startTime - since,
        })),
    }),
    { channel, since },
  );

test("100-channel tuner meets surf latency and latest-request-wins gates", async ({ page }) => {
  expect(tunerManifest("ended", 1)).toContain("#EXT-X-ENDLIST");
  expect(tunerManifest("live", 4)).not.toContain("#EXT-X-ENDLIST");
  expect(tunerManifest("live", 4)).not.toContain("#EXT-X-PLAYLIST-TYPE:VOD");
  await installFrameClock(page);
  const backend = await installTunerBackend(page);

  // Prove the engine can cold-start and decode the representative H.264 stream, but keep decoder
  // bootstrap out of the surf percentile. The real-runtime gate owns cold boot timing; this gate
  // owns an already-running tuner and never retries a black start.
  await page.goto(`/channels/${channelId(1)}/watch`);
  await waitForDecodedFrame(page);
  await expect(page.locator("video")).toHaveCount(1);
  await expect.poll(() => page.locator("video").evaluate((element) => element.ended)).toBe(true);
  await waitForAdjacentWarm(page, 1);

  // Arbitrary prepared tune: navigate the already-running app to a non-adjacent Channel. Measure
  // route request to a genuinely decoded frame, including SPA/API/HLS work but excluding the cold
  // document + decoder bootstrap proven above and owned by the real-runtime gate.
  const arbitrary: number[] = [];
  const arbitraryTraces: string[] = [];
  for (const number of [3, 17, 29, 41, 53, 67, 79, 91, 8, 50, 62, 74, 86, 98, 12, 24, 36, 48, 60, 72]) {
    const sample = await tuneInApp(page, channelId(number));
    arbitrary.push(sample.duration);
    arbitraryTraces.push(`${number}:${sample.duration.toFixed(0)}[${sample.trace}]`);
    await expect(page.locator("video")).toHaveCount(1);
    // Keep arbitrary samples independent. Rapid sequential intent has its own adjacent and burst
    // gates below; this percentile starts after the previous Channel's bounded hot set has settled.
    await waitForAdjacentWarm(page, number);
  }
  expect(
    p95(arbitrary),
    `arbitrary prepared p95: ${p95(arbitrary).toFixed(1)}ms; samples: ${arbitrary
      .map((sample) => sample.toFixed(1))
      .join(", ")}; traces: ${arbitraryTraces.join(" | ")}`,
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
  await channelUp(page);
  await expect(page.getByRole("status")).toContainText("CH 51");
  await expect(video).toHaveAttribute("poster", /^data:image\/png;base64,/);
  await page.waitForFunction(
    (count) => performance.getEntriesByName("loomarr:tune:request-to-first-frame").length > count,
    heldFrameCount,
  );
  await expect(video).not.toHaveAttribute("poster", /^data:image\/png;base64,/);

  // Reset to the middle so the measured adjacent run remains the same 50 → 70 sample.
  const resetWarmCounts = await adjacentWarmCounts(page, 50);
  await page.goto(`/channels/${channelId(50)}/watch`);
  await waitForDecodedFrame(page);
  // Warm proof uses an absolute timestamp, so this full-document reset cannot make a new mark look
  // older merely because performance.startTime restarted from zero.
  await waitForAdjacentWarm(page, 50, resetWarmCounts);

  await page.evaluate(() => {
    performance.clearMeasures("loomarr:tune:request-to-osd");
    performance.clearMeasures("loomarr:tune:request-to-first-frame");
  });
  const osd: number[] = [];
  const adjacentFrames: number[] = [];
  const adjacentTraces: string[] = [];
  let current = 50;
  for (let sample = 0; sample < 20; sample++) {
    const target = current === 100 ? 1 : current + 1;
    const id = channelId(target);
    const targetMints = backend.state.playURLMints.filter((candidate) => candidate === id).length;
    const next = target === 100 ? 1 : target + 1;
    const nextWarmAt = await latestWarmAt(page, next);

    const previousFrames = await page.evaluate(
      () => performance.getEntriesByName("loomarr:tune:request-to-first-frame").length,
    );
    const started = await page.evaluate(() => performance.now());
    await channelUp(page);
    await expect(page).toHaveURL(new RegExp(`/channels/${id}/watch$`));
    try {
      await page.waitForFunction(
        (count) => performance.getEntriesByName("loomarr:tune:request-to-first-frame").length > count,
        previousFrames,
        { timeout: 10_000 },
      );
    } catch (error) {
      const playback = await playbackSnapshot(page, id, started);
      throw new Error(`${id} produced no target frame: ${JSON.stringify(playback)}`, { cause: error });
    }

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
    await waitForTargetPlaying(page, id, started);
    osd.push(timing.osd);
    adjacentFrames.push(timing.frame);
    adjacentTraces.push(
      await page.evaluate(
        ({ id, started }) => {
          const frame = performance.getEntriesByName("loomarr:tune:request-to-first-frame").at(-1) as
            | PerformanceMeasure
            | undefined;
          const attemptId = (frame?.detail as { attemptId?: number } | undefined)?.attemptId;
          const phases = attemptId
            ? performance
                .getEntriesByType("mark")
                .filter((entry) => entry.name.startsWith(`loomarr:tune:${attemptId}:`))
                .map((entry) => `${entry.name.split(":").at(-1)}@${(entry.startTime - started).toFixed(0)}`)
            : [];
          const resources = performance
            .getEntriesByType("resource")
            .filter((entry) => entry.startTime >= started)
            .filter((entry) => /master\.m3u8|init\.mp4|segment\.m4s/.test(entry.name))
            .map(
              (entry) =>
                `${new URL(entry.name).pathname.split("/").at(-1)}:${(entry.startTime - started).toFixed(0)}-${(entry.responseEnd - started).toFixed(0)}`,
            );
          const events = (
            window as Window & {
              __loomarrMediaEvents?: Array<{ at: number; channel: string; type: string }>;
            }
          ).__loomarrMediaEvents
            ?.filter((event) => event.at >= started && event.channel === id)
            .map((event) => `${event.type}@${(event.at - started).toFixed(0)}`);
          return [...phases, ...resources, ...(events ?? [])].join(" ");
        },
        { id, started },
      ),
    );
    expect(backend.state.playURLMints.filter((candidate) => candidate === id)).toHaveLength(targetMints);
    await expect(page.locator("video")).toHaveCount(1);
    // The frame makes this Channel active; its post-frame warmer must consume the next Channel's
    // response bodies before the following measured click. Observe the controller's completion
    // seam rather than requiring another network request: a valid browser-cache hit is ready too.
    await expect.poll(() => latestWarmAt(page, next)).toBeGreaterThan(nextWarmAt);
    current = target;
  }

  expect(p95(osd), `OSD p95: ${p95(osd).toFixed(1)}ms`).toBeLessThan(100);
  expect(
    p95(adjacentFrames),
    `prepared adjacent first-frame p95: ${p95(adjacentFrames).toFixed(1)}ms; samples: ${adjacentFrames
      .map((sample) => sample.toFixed(1))
      .join(", ")}; traces: ${adjacentTraces.join(" | ")}`,
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
  await channelUp(page);
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
