const assert = require("node:assert/strict");
const { test } = require("node:test");
const {
  addMemorySafeAndroidBuild,
} = require("./with-memory-safe-android-build.cjs");

test("adds bounded CMake pools to every generated Android subproject", () => {
  const generated = addMemorySafeAndroidBuild("buildscript {\n}\n\nallprojects {\n}\n");

  assert.match(generated, /System\.getenv\("LOOMARR_ANDROID_NATIVE_JOBS"\) \?: "1"/);
  assert.match(generated, /\^\[1-9\]\[0-9\]\*\$/);
  assert.match(
    generated,
    /CMAKE_JOB_POOLS=loomarr_compile=\$\{loomarrAndroidNativeJobs\};loomarr_link=\$\{loomarrAndroidNativeJobs\}/,
  );
  assert.match(generated, /CMAKE_JOB_POOL_COMPILE=loomarr_compile/);
  assert.match(generated, /CMAKE_JOB_POOL_LINK=loomarr_link/);
  assert.match(generated, /subproject\.pluginManager\.withPlugin\(pluginId\)/);
  assert.match(generated, /com\.android\.library/);
  assert.equal(addMemorySafeAndroidBuild(generated), generated);
});

test("fails closed when Expo's generated root build shape changes", () => {
  assert.throws(
    () => addMemorySafeAndroidBuild("buildscript {\n}\n"),
    /Could not find Android allprojects/,
  );
});
