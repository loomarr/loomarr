import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { describe, it } from "node:test";

const workspace = JSON.parse(readFileSync(new URL("../package.json", import.meta.url), "utf8"));
const androidBuild = readFileSync(new URL("./build-android-client.sh", import.meta.url), "utf8");

describe("shared client resource bounds", () => {
  it("bundles mobile and TV clients one at a time", () => {
    assert.match(workspace.scripts["clients:check"], /turbo run bundle --concurrency=1/);
  });

  it("keeps native Android compilation below interactive-desktop limits", () => {
    assert.match(androidBuild, /LOOMARR_ANDROID_MEMORY_MAX:-3G/);
    assert.match(androidBuild, /LOOMARR_ANDROID_GRADLE_HEAP:-1024m/);
    assert.match(androidBuild, /--nice=10/);
    assert.match(androidBuild, /\/usr\/bin\/ionice -c 3/);
    assert.match(androidBuild, /CPUQuota=200%/);
    assert.match(androidBuild, /--max-workers=1/);
  });
});
