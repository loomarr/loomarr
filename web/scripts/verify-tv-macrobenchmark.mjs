#!/usr/bin/env node
import { readFileSync } from "node:fs";
import { pathToFileURL } from "node:url";

const P95_LIMIT_MS = 32;
const P99_LIMIT_MS = 50;
const FROZEN_FRAME_MS = 700;

const numericSamples = (value) => {
  if (typeof value === "number" && Number.isFinite(value)) return [value];
  if (!Array.isArray(value)) return [];
  return value.flatMap(numericSamples);
};

const percentile = (samples, fraction) => {
  const ordered = [...samples].sort((a, b) => a - b);
  if (ordered.length === 0) throw new Error("frameDurationCpuMs has no raw frame samples");
  const rank = Math.max(0, Math.ceil(fraction * ordered.length) - 1);
  return ordered[rank];
};

const benchmarkName = (benchmark) => `${benchmark.className ?? ""}#${benchmark.name ?? ""}`;

const verifyTvMacrobenchmark = (report) => {
  const benchmarks = report.benchmarks;
  if (!Array.isArray(benchmarks)) throw new Error("benchmark report has no benchmarks array");
  const benchmark = benchmarks.find((entry) => benchmarkName(entry).includes("guideNavigation"));
  if (!benchmark) throw new Error("benchmark report has no guideNavigation result");
  const metric = benchmark.sampledMetrics?.frameDurationCpuMs;
  if (!metric) throw new Error("guideNavigation has no frameDurationCpuMs metric");
  const samples = numericSamples(metric.runs);
  if (samples.length === 0) throw new Error("frameDurationCpuMs has no raw frame samples");
  const p95 = typeof metric.P95 === "number" ? metric.P95 : percentile(samples, 0.95);
  const p99 = typeof metric.P99 === "number" ? metric.P99 : percentile(samples, 0.99);
  const frozenFrames = samples.filter((sample) => sample >= FROZEN_FRAME_MS).length;
  const result = { frameCount: samples.length, frozenFrames, p95, p99 };
  const failures = [];
  if (p95 > P95_LIMIT_MS) failures.push(`p95 ${p95.toFixed(2)} ms exceeds ${P95_LIMIT_MS} ms`);
  if (p99 > P99_LIMIT_MS) failures.push(`p99 ${p99.toFixed(2)} ms exceeds ${P99_LIMIT_MS} ms`);
  if (frozenFrames > 0) failures.push(`${frozenFrames} frame(s) were ${FROZEN_FRAME_MS} ms or longer`);
  if (failures.length > 0) throw new Error(failures.join("; "));
  return result;
};

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  const reportPath = process.argv[2];
  if (!reportPath) {
    console.error("usage: verify-tv-macrobenchmark.mjs <benchmarkData.json>");
    process.exit(2);
  }
  try {
    const result = verifyTvMacrobenchmark(JSON.parse(readFileSync(reportPath, "utf8")));
    console.log(
      `Shield Guide Macrobenchmark passed: p95=${result.p95.toFixed(2)} ms, ` +
        `p99=${result.p99.toFixed(2)} ms, frozen=${result.frozenFrames}, frames=${result.frameCount}`,
    );
  } catch (error) {
    console.error(`Shield Guide Macrobenchmark failed: ${error.message}`);
    process.exit(1);
  }
}

export { verifyTvMacrobenchmark };
