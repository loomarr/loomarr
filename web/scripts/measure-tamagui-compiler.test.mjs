import assert from "node:assert/strict";
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { test } from "node:test";
import { bundleBytes, compareBundleBytes } from "./measure-tamagui-compiler.mjs";

test("counts only exported JavaScript and Hermes bytecode", () => {
  const root = mkdtempSync(path.join(tmpdir(), "loomarr-compiler-measure-test-"));
  try {
    mkdirSync(path.join(root, "nested"));
    writeFileSync(path.join(root, "runtime.hbc"), Buffer.alloc(10));
    writeFileSync(path.join(root, "nested", "chunk.js"), Buffer.alloc(7));
    writeFileSync(path.join(root, "nested", "chunk.js.map"), Buffer.alloc(100));
    writeFileSync(path.join(root, "metadata.json"), Buffer.alloc(100));
    assert.equal(bundleBytes(root), 17);
  } finally {
    rmSync(root, { force: true, recursive: true });
  }
});

test("reports byte and percentage regression without rounding the source bytes", () => {
  assert.deepEqual(compareBundleBytes("tv", 2_496_641, 2_517_056), {
    app: "tv",
    compilerBytes: 2_517_056,
    deltaBytes: 20_415,
    deltaPercent: 0.818,
    runtimeBytes: 2_496_641,
  });
});
