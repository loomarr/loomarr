const assert = require("node:assert/strict");
const { existsSync, mkdtempSync, readFileSync, rmSync } = require("node:fs");
const { tmpdir } = require("node:os");
const path = require("node:path");
const { test } = require("node:test");
const {
  addMacrobenchmarkSettings,
  addProfileableApplication,
  guideNavigationBenchmark,
  macrobenchmarkBuildGradle,
  writeMacrobenchmarkModule,
} = require("./with-tv-macrobenchmark.cjs");

test("adds the generated Macrobenchmark module exactly once", () => {
  const generated = addMacrobenchmarkSettings("include ':app'\n");
  assert.match(generated, /with-tv-macrobenchmark[\s\S]*include ':macrobenchmark'/);
  assert.equal(addMacrobenchmarkSettings(generated), generated);
});

test("makes the release-like target profileable without making it debuggable", () => {
  const manifest = { manifest: { application: [{ $: { "android:name": ".MainApplication" } }] } };
  const generated = addProfileableApplication(manifest);
  assert.deepEqual(generated.manifest.application[0].profileable, [
    { $: { "android:shell": "true" } },
  ]);
  assert.equal(addProfileableApplication(generated).manifest.application[0].profileable.length, 1);
});

test("pins a separate release-like AndroidX benchmark and a paired Guide traversal", () => {
  assert.match(macrobenchmarkBuildGradle, /id 'com\.android\.test'/);
  assert.match(macrobenchmarkBuildGradle, /matchingFallbacks = \['release'\]/);
  assert.match(macrobenchmarkBuildGradle, /benchmark-macro-junit4:1\.4\.1/);
  assert.match(macrobenchmarkBuildGradle, /uiautomator:2\.4\.0/);
  assert.match(guideNavigationBenchmark, /CompilationMode\.Ignore\(\)/);
  assert.match(guideNavigationBenchmark, /By\.desc\("Programme guide"\)/);
  assert.match(guideNavigationBenchmark, /repeat\(12\)/);
  assert.match(guideNavigationBenchmark, /FrameTimingMetric\(\)/);
});

test("writes the complete generated module", () => {
  const root = mkdtempSync(path.join(tmpdir(), "loomarr-tv-macrobenchmark-"));
  try {
    writeMacrobenchmarkModule(root);
    const moduleRoot = path.join(root, "android", "macrobenchmark");
    assert.equal(readFileSync(path.join(moduleRoot, "build.gradle"), "utf8"), macrobenchmarkBuildGradle);
    assert.equal(
      readFileSync(
        path.join(
          moduleRoot,
          "src/main/java/media/loomarr/tv/macrobenchmark/GuideNavigationBenchmark.kt",
        ),
        "utf8",
      ),
      guideNavigationBenchmark,
    );
    assert.ok(existsSync(path.join(moduleRoot, "src/main/AndroidManifest.xml")));
  } finally {
    rmSync(root, { force: true, recursive: true });
  }
});
