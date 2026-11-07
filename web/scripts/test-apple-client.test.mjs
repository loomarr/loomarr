import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { readFileSync } from "node:fs";
import { describe, it } from "node:test";
import { fileURLToPath } from "node:url";

const script = readFileSync(new URL("test-apple-client.sh", import.meta.url), "utf8");
const xcconfig = readFileSync(new URL("apple-simulator.xcconfig", import.meta.url), "utf8");
const cacheXcconfig = readFileSync(new URL("apple-compilation-cache.xcconfig", import.meta.url), "utf8");

describe("Apple simulator verifier", () => {
  it("builds without physical-device discovery and explicitly installs the selected simulator", () => {
    assert.match(script, /--device generic/);
    assert.doesNotMatch(script, /--device "\$\{simulator_id\}"/);
    assert.match(script, /xcrun simctl install "\$\{simulator_id\}" "\$\{BUILD_DIR\}\/\$\{SCHEME\}\.app"/);
    assert.ok(script.indexOf("xcrun simctl install") < script.indexOf("xcrun simctl launch"));
    assert.match(script, /args\+=\("ARCHS=\$\(uname -m\)"\)/);
  });

  it("fails closed unless the iOS 27 scene-lifecycle toolchain is active", () => {
    assert.match(script, /xcode_version.*\^27\\\./s);
    assert.match(script, /requires Xcode 27\.x/);
  });

  it("generates from the template inside the pinned Expo package", () => {
    assert.match(script, /require\.resolve\('expo\/package\.json'\)/);
    assert.equal(script.match(/--template "\$\{EXPO_TEMPLATE\}"/g)?.length, 2);
  });

  it("builds only the active simulator architecture", () => {
    const settings = xcconfig
      .split("\n")
      .map((line) => line.trim())
      .filter((line) => line !== "" && !line.startsWith("//"));

    assert.deepEqual(settings, ["ONLY_ACTIVE_ARCH = YES"]);
    assert.equal(script.match(/XCODE_XCCONFIG_FILE="\$xcconfig"/g)?.length, 2);
  });

  it("enables only Xcode's compilation CAS in warm mode", () => {
    const settings = cacheXcconfig
      .split("\n")
      .map((line) => line.trim())
      .filter((line) => line !== "" && !line.startsWith("//"));

    assert.deepEqual(settings, [
      '#include "apple-simulator.xcconfig"',
      "COMPILATION_CACHE_ENABLE_CACHING = YES",
      "COMPILATION_CACHE_ENABLE_DIAGNOSTIC_REMARKS = YES",
      "COMPILATION_CACHE_CAS_PATH = $(LOOMARR_APPLE_CACHE_STORE)",
      "SWIFT_ENABLE_EXPLICIT_MODULES = YES",
    ]);
  });

  it("fails when the built artifact does not contain exactly the host architecture", () => {
    assert.match(script, /APP_ARCHS="\$\(xcrun lipo -archs "\$\{APP_BINARY\}"\)"/);
    assert.match(script, /if \[\[ "\$\{APP_ARCHS\}" != "\$\{HOST_ARCH\}" \]\]; then/);
  });
});

describe("Apple simulator boot after the build", () => {
  const boot = script.match(/^boot_simulator_after_build\(\) \{\n[\s\S]*?^\}\n/m)?.[0];

  // Runs the script's real function against stand-in commands, and reports its status, how many
  // boots it tried, and how many times it restarted CoreSimulator.
  const bootWith = (failures) => {
    const result = spawnSync(
      "bash",
      [
        "-c",
        `set -euo pipefail
simulator_id=test-device
attempts=0
restarts=0
xcrun() {
  [[ "$2" == bootstatus ]] || return 0
  attempts=$((attempts + 1))
  ((attempts > ${failures}))
}
killall() { restarts=$((restarts + 1)); }
sleep() { :; }
${boot}
status=0
boot_simulator_after_build 2>/dev/null || status=$?
printf '%s %s %s' "$status" "$attempts" "$restarts"`,
      ],
      { encoding: "utf8", timeout: 30_000 },
    );
    assert.equal(result.status, 0, result.error?.message ?? result.stderr);
    return result.stdout;
  };

  it("leaves CoreSimulator alone when the device boots", () => {
    assert.equal(bootWith(0), "0 1 0");
  });

  it("restarts CoreSimulator and boots again after a failure", () => {
    assert.equal(bootWith(1), "0 2 1");
    assert.equal(bootWith(2), "0 3 2");
  });

  it("still fails the job when the device never boots", () => {
    assert.equal(bootWith(3), "1 3 3");
  });

  it("boots before it installs", () => {
    assert.ok(script.indexOf("\nboot_simulator_after_build\n") < script.indexOf("xcrun simctl install"));
  });
});

describe("Apple launch failure evidence", () => {
  it("retains only current matching crashes and bounds failed diagnostic commands", () => {
    const result = spawnSync(
      "python3",
      [fileURLToPath(new URL("apple-launch-diagnostics-test.py", import.meta.url))],
      {
        encoding: "utf8",
        timeout: 30_000,
        env: { ...process.env, PYTHONDONTWRITEBYTECODE: "1" },
      },
    );
    assert.equal(result.status, 0, result.error?.message ?? result.stderr);
  });
});
