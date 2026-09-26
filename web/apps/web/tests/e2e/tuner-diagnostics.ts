import os from "node:os";
import type { Page, TestInfo } from "@playwright/test";

// Failure-only evidence for a tuner start that stalls before any fragment is requested (#1443).
// Everything here is collected passively during the run and attached to the report only when the
// test fails, so a passing run pays a bounded in-memory log and nothing else.

const HLS_LOG_LIMIT = 4_000;

interface NetworkEntry {
  at: number;
  event: "request" | "response" | "failed";
  url: string;
  detail?: string;
}

interface StartDiagnostics {
  attachOnFailure(testInfo: TestInfo): Promise<void>;
}

const loadSnapshot = () => ({
  loadavg: os.loadavg().map((value) => Number(value.toFixed(2))),
  cpus: os.cpus().length,
  freeMemMB: Math.round(os.freemem() / 1_048_576),
});

const installStartDiagnostics = async (page: Page): Promise<StartDiagnostics> => {
  const origin = Date.now();
  const network: NetworkEntry[] = [];
  const consoleLines: string[] = [];
  const beforeRun = loadSnapshot();
  const trim = (url: string) => {
    try {
      const parsed = new URL(url);
      return parsed.pathname + parsed.search;
    } catch {
      return url;
    }
  };
  page.on("request", (request) => {
    if (request.resourceType() === "document") return;
    network.push({ at: Date.now() - origin, event: "request", url: trim(request.url()) });
  });
  page.on("response", (response) =>
    network.push({
      at: Date.now() - origin,
      event: "response",
      url: trim(response.url()),
      detail: String(response.status()),
    }),
  );
  page.on("requestfailed", (request) =>
    network.push({
      at: Date.now() - origin,
      event: "failed",
      url: trim(request.url()),
      detail: request.failure()?.errorText,
    }),
  );
  page.on("console", (message) => {
    if (consoleLines.length < 500) consoleLines.push(`${message.type()}: ${message.text()}`);
  });
  page.on("pageerror", (error) => consoleLines.push(`pageerror: ${error.message}`));

  await page.addInitScript((limit) => {
    const entries: Array<{ at: number; controller: number; level: string; text: string }> = [];
    Object.defineProperty(window, "__loomarrHlsLog", { value: entries, configurable: true });
    Object.defineProperty(window, "__loomarrHlsDebug", {
      configurable: true,
      value: {
        push(controller: number, level: string, args: unknown[]) {
          // hls.js formats its own log lines; keep them as plain strings so the entry survives
          // JSON serialisation even when an argument is a DOM or class instance.
          const text = args
            .map((arg) =>
              typeof arg === "string"
                ? arg
                : (() => {
                    try {
                      return JSON.stringify(arg);
                    } catch {
                      return String(arg);
                    }
                  })(),
            )
            .join(" ");
          if (entries.length < limit) entries.push({ at: performance.now(), controller, level, text });
        },
      },
    });
  }, HLS_LOG_LIMIT);

  return {
    async attachOnFailure(testInfo) {
      if (testInfo.status === testInfo.expectedStatus) return;
      const player = await page
        .evaluate(async () => {
          const video = document.querySelector("video");
          const ranges = (list: TimeRanges) =>
            Array.from({ length: list.length }, (_, index) => [list.start(index), list.end(index)]);
          // A throttled or hidden page starves timers and rAF; count ticks over a fixed window so a
          // slow-runner explanation is separated from a player that is simply waiting.
          const ticks = await new Promise<number>((resolve) => {
            let count = 0;
            const timer = window.setInterval(() => count++, 10);
            window.setTimeout(() => {
              window.clearInterval(timer);
              resolve(count);
            }, 500);
          });
          return {
            now: performance.now(),
            visibility: document.visibilityState,
            hasFocus: document.hasFocus(),
            timerTicksIn500ms: ticks,
            video: video && {
              readyState: video.readyState,
              networkState: video.networkState,
              currentTime: video.currentTime,
              paused: video.paused,
              seeking: video.seeking,
              buffered: ranges(video.buffered),
              seekable: ranges(video.seekable),
              currentSrc: video.currentSrc,
              hasMediaSourceObjectURL: video.currentSrc.startsWith("blob:"),
              error: video.error && { code: video.error.code, message: video.error.message },
            },
            hlsLog: (window as Window & { __loomarrHlsLog?: unknown[] }).__loomarrHlsLog ?? [],
            resources: performance
              .getEntriesByType("resource")
              .filter((entry) => /play-url|m3u8|init\.mp4|\.m4s|\.ts(?:$|\?)/.test(entry.name))
              .map((entry) => ({
                name: new URL(entry.name).pathname,
                start: entry.startTime,
                end: entry.responseEnd,
              })),
          };
        })
        .catch((error: unknown) => ({ evaluateError: String(error) }));
      const report = {
        project: testInfo.project.name,
        workerIndex: testInfo.workerIndex,
        testElapsedMs: Date.now() - origin,
        loadAtStart: beforeRun,
        loadAtFailure: loadSnapshot(),
        runner: { platform: process.platform, arch: process.arch, nodeVersion: process.version },
        player,
        network,
        console: consoleLines,
      };
      await testInfo.attach("start-diagnostics.json", {
        body: JSON.stringify(report, null, 2),
        contentType: "application/json",
      });
    },
  };
};

export type { StartDiagnostics };
export { installStartDiagnostics };
