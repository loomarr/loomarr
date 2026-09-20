import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

test("uses a prototype application identity", async () => {
  const config = JSON.parse(await readFile(new URL("../app.json", import.meta.url), "utf8"));
  assert.match(config.expo.ios.bundleIdentifier, /\.prototype$/);
  assert.match(config.expo.android.package, /\.prototype$/);
  assert.equal(config.expo.orientation, "default");
  assert.ok(config.expo.plugins.includes("../../scripts/with-memory-safe-android-build.cjs"));
});

test("does not autolink unused animation native modules", async () => {
  const manifest = JSON.parse(await readFile(new URL("../package.json", import.meta.url), "utf8"));
  const excluded = new Set(manifest.expo.autolinking.exclude);

  for (const dependency of ["react-native-reanimated", "react-native-worklets"]) {
    assert.equal(manifest.dependencies[dependency], undefined);
    assert.ok(excluded.has(dependency));
  }
});
