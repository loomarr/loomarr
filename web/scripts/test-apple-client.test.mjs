import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { describe, it } from "node:test";

const script = readFileSync(new URL("test-apple-client.sh", import.meta.url), "utf8");
const xcconfig = readFileSync(new URL("apple-simulator.xcconfig", import.meta.url), "utf8");
const cacheXcconfig = readFileSync(new URL("apple-compilation-cache.xcconfig", import.meta.url), "utf8");

describe("Apple simulator verifier", () => {
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
