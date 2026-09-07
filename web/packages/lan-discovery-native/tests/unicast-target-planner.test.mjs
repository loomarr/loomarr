import { execFileSync } from "node:child_process";
import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

const root = new URL("../", import.meta.url);
const source = (name) => new URL(`android/src/main/java/media/loomarr/tv/discovery/${name}`, root).pathname;

describe("native Android unicast planning", () => {
  it("executes deterministic bounded target planning and paced cancellation", () => {
    const output = mkdtempSync(join(tmpdir(), "loomarr-unicast-test-"));
    try {
      execFileSync("javac", ["-d", output, source("LocalNetworkSelector.java"), source("UnicastTargetPlanner.java"), source("UnicastRetryPolicy.java"), source("UnicastSweep.java"), new URL("native/UnicastTargetPlannerTest.java", import.meta.url).pathname]);
      execFileSync("java", ["-cp", output, "media.loomarr.tv.discovery.UnicastTargetPlannerTest"]);
    } finally {
      rmSync(output, { recursive: true, force: true });
    }
    expect(true).toBe(true);
  });
});
