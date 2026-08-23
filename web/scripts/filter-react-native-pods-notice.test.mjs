import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { describe, it } from "node:test";

const filter = (input) =>
  spawnSync("awk", ["-f", "filter-react-native-pods-notice.awk"], {
    cwd: import.meta.dirname,
    encoding: "utf8",
    input,
  });

describe("React Native CocoaPods notice filter", () => {
  it("removes only the known deprecation banner", () => {
    const result = filter(`before
==================== DEPRECATION NOTICE =====================
Calling \`pod install\` directly is deprecated in React Native
because we are moving away from Cocoapods toward alternative
solutions to build the project.
* If you are using Expo, please run:
\`npx expo run:ios\`
* If you are using the Community CLI, please run:
\`yarn ios\`
=============================================================
after
`);

    assert.equal(result.status, 0);
    assert.equal(result.stdout, "before\nafter\n");
  });

  it("preserves unrelated warnings", () => {
    const result = filter("warning: pod install failed\n");

    assert.equal(result.status, 0);
    assert.equal(result.stdout, "warning: pod install failed\n");
  });

  it("fails closed when the banner is incomplete", () => {
    const result = filter("==================== DEPRECATION NOTICE =====================\ntruncated\n");

    assert.equal(result.status, 1);
  });
});
