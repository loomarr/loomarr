const assert = require("node:assert/strict");
const { test } = require("node:test");
const {
  addMemorySafeAndroidBuild,
} = require("./with-memory-safe-android-build.cjs");

test("adds bounded CMake pools to Expo's generated defaultConfig", () => {
  const generated = addMemorySafeAndroidBuild("android {\n    defaultConfig {\n    }\n}\n");

  assert.match(generated, /CMAKE_JOB_POOLS=loomarr_compile=1;loomarr_link=1/);
  assert.match(generated, /CMAKE_JOB_POOL_COMPILE=loomarr_compile/);
  assert.match(generated, /CMAKE_JOB_POOL_LINK=loomarr_link/);
  assert.equal(addMemorySafeAndroidBuild(generated), generated);
});

test("fails closed when Expo's generated build shape changes", () => {
  assert.throws(
    () => addMemorySafeAndroidBuild("android {\n}\n"),
    /Could not find Android defaultConfig/,
  );
});
