import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { describe, it } from "node:test";

const script = readFileSync(new URL("test-apple-client.sh", import.meta.url), "utf8");
const xcconfig = readFileSync(new URL("apple-simulator.xcconfig", import.meta.url), "utf8");

describe("Apple simulator verifier", () => {
  it("builds only the active simulator architecture", () => {
    const settings = xcconfig
      .split("\n")
      .map((line) => line.trim())
      .filter((line) => line !== "" && !line.startsWith("//"));

    assert.deepEqual(settings, ["ONLY_ACTIVE_ARCH = YES"]);
    assert.equal(script.match(/XCODE_XCCONFIG_FILE="\$\{APPLE_SIMULATOR_XCCONFIG\}"/g)?.length, 2);
  });

  it("fails when the built artifact does not contain exactly the host architecture", () => {
    assert.match(script, /APP_ARCHS="\$\(xcrun lipo -archs "\$\{APP_BINARY\}"\)"/);
    assert.match(script, /if \[\[ "\$\{APP_ARCHS\}" != "\$\{HOST_ARCH\}" \]\]; then/);
  });
});
