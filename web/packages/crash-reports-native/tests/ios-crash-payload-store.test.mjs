import { execFileSync } from "node:child_process";
import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

const root = new URL("../", import.meta.url);

// Swift compiles only on macOS; the Apple mobile CI job runs this file on its Xcode runner.
describe.runIf(process.platform === "darwin")("native iOS crash payload store", () => {
  it("persists, bounds and removes MetricKit payloads", () => {
    const output = mkdtempSync(join(tmpdir(), "loomarr-ios-crash-store-test-"));
    try {
      const binary = join(output, "crash-store-tests");
      execFileSync(
        "xcrun",
        [
          "swiftc",
          "-parse-as-library",
          "-o",
          binary,
          new URL("ios/CrashPayloadStore.swift", root).pathname,
          new URL("native-ios/CrashPayloadStoreTests.swift", import.meta.url).pathname,
        ],
        { timeout: 120000 },
      );
      expect(execFileSync(binary, { encoding: "utf8", timeout: 10000 })).toContain(
        "ios crash payload store: ok",
      );
    } finally {
      rmSync(output, { recursive: true, force: true });
    }
  }, 150000);
});
