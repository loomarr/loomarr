import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { describe, it } from "node:test";

const workspace = JSON.parse(readFileSync(new URL("../package.json", import.meta.url), "utf8"));
const androidBuild = readFileSync(new URL("./build-android-client.sh", import.meta.url), "utf8");
const androidTvEmulator = readFileSync(
  new URL("../../scripts/run-android-tv-emulator.sh", import.meta.url),
  "utf8",
);

describe("shared client resource bounds", () => {
  it("bundles mobile and TV clients one at a time", () => {
    assert.match(workspace.scripts["clients:check"], /turbo run bundle --concurrency=1/);
  });

  it("keeps native Android compilation below interactive-desktop limits", () => {
    assert.match(androidBuild, /LOOMARR_ANDROID_MEMORY_MAX:-3G/);
    assert.match(androidBuild, /LOOMARR_ANDROID_GRADLE_HEAP:-1024m/);
    assert.match(androidBuild, /LOOMARR_ANDROID_MIN_AVAILABLE_KB:-6291456/);
    assert.match(androidBuild, /refusing native build/);
    assert.match(androidBuild, /--nice=10/);
    assert.match(androidBuild, /\/usr\/bin\/ionice -c 2 -n 7/);
    assert.match(androidBuild, /CPUQuota=200%/);
    assert.match(androidBuild, /--max-workers=1/);
  });

  it("keeps the Android TV emulator below interactive-desktop limits", () => {
    assert.match(androidTvEmulator, /LOOMARR_TV_EMULATOR_GPU:-auto/);
    assert.match(androidTvEmulator, /LOOMARR_TV_EMULATOR_MEMORY_MAX:-3584M/);
    assert.match(androidTvEmulator, /LOOMARR_TV_EMULATOR_MIN_AVAILABLE_KB:-8388608/);
    assert.match(androidTvEmulator, /LOOMARR_TV_EMULATOR_BOOT_TIMEOUT_SECONDS:-120/);
    assert.match(androidTvEmulator, /refusing emulator start/);
    assert.match(androidTvEmulator, /MemoryMax=/);
    assert.match(androidTvEmulator, /CPUQuota=200%/);
    assert.match(androidTvEmulator, /timeout 5 "\$adb_bin"/);
    assert.doesNotMatch(androidTvEmulator, /-gpu swiftshader_indirect/);
  });
});
