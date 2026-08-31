import assert from "node:assert/strict";
import { test } from "node:test";
import { verifyTvMacrobenchmark } from "./verify-tv-macrobenchmark.mjs";

const report = (runs, percentiles = {}) => ({
  benchmarks: [
    {
      className: "media.loomarr.tv.macrobenchmark.GuideNavigationBenchmark",
      name: "guideNavigation",
      sampledMetrics: { frameDurationCpuMs: { runs, ...percentiles } },
    },
  ],
});

test("accepts Shield Guide frames within every adoption limit", () => {
  assert.deepEqual(
    verifyTvMacrobenchmark(
      report(
        [
          [8, 12, 16],
          [20, 24, 30],
        ],
        { P95: 30, P99: 30 },
      ),
    ),
    {
      frameCount: 6,
      frozenFrames: 0,
      p95: 30,
      p99: 30,
    },
  );
});

test("rejects p95 or p99 regression even without a frozen frame", () => {
  assert.throws(
    () => verifyTvMacrobenchmark(report([[8, 12, 60]], { P95: 33, P99: 60 })),
    /p95 33\.00 ms exceeds 32 ms; p99 60\.00 ms exceeds 50 ms/,
  );
});

test("rejects every frame at or above the frozen-frame boundary", () => {
  assert.throws(
    () => verifyTvMacrobenchmark(report([[8, 12, 700]], { P95: 12, P99: 12 })),
    /1 frame\(s\) were 700 ms or longer/,
  );
});

test("fails closed when raw frame samples or the named benchmark are missing", () => {
  assert.throws(() => verifyTvMacrobenchmark(report([], { P95: 1, P99: 1 })), /no raw frame samples/);
  assert.throws(() => verifyTvMacrobenchmark({ benchmarks: [] }), /no guideNavigation result/);
});
