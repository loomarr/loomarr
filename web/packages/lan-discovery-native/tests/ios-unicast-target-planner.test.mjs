import { execFileSync } from "node:child_process";
import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

const root = new URL("../", import.meta.url);

// Swift compiles only on macOS; the Apple mobile CI job runs this file on its Xcode runner.
describe.runIf(process.platform === "darwin")("native iOS unicast planning", () => {
  it("executes deterministic bounded target planning and paced cancellation", () => {
    const output = mkdtempSync(join(tmpdir(), "loomarr-ios-unicast-test-"));
    try {
      const binary = join(output, "planner-tests");
      execFileSync(
        "xcrun",
        [
          "swiftc",
          "-parse-as-library",
          "-o",
          binary,
          new URL("ios/UnicastTargetPlanner.swift", root).pathname,
          new URL("native-ios/UnicastTargetPlannerTests.swift", import.meta.url).pathname,
        ],
        { timeout: 120000 },
      );
      expect(execFileSync(binary, { encoding: "utf8", timeout: 10000 })).toContain("ios unicast planner: ok");
    } finally {
      rmSync(output, { recursive: true, force: true });
    }
  }, 150000);
});
