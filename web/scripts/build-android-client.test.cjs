const assert = require("node:assert/strict");
const { mkdtempSync, readFileSync, rmSync, writeFileSync } = require("node:fs");
const { tmpdir } = require("node:os");
const path = require("node:path");
const { spawnSync } = require("node:child_process");
const test = require("node:test");

const executable = (file, body) => writeFileSync(file, body, { mode: 0o755 });

test("forwards the configured public Loomarr URL into the bounded Android build", () => {
  const bin = mkdtempSync(path.join(tmpdir(), "loomarr-android-build-test-"));
  try {
    executable(path.join(bin, "systemctl"), "#!/bin/sh\nexit 0\n");
    executable(path.join(bin, "systemd-run"), "#!/bin/sh\nprintf '%s\\n' \"$@\"\n");

    const result = spawnSync("bash", [path.join(__dirname, "build-android-client.sh"), "tv"], {
      encoding: "utf8",
      env: {
        ANDROID_HOME: "/android-sdk",
        EXPO_PUBLIC_LOOMARR_URL: "http://192.0.2.10:18080",
        LOOMARR_ANDROID_MIN_AVAILABLE_KB: "0",
        PATH: `${bin}:${process.env.PATH}`,
      },
    });

    assert.equal(result.status, 0, result.stderr);
    assert.match(result.stdout, /^EXPO_PUBLIC_LOOMARR_URL=http:\/\/192\.0\.2\.10:18080$/m);
    assert.match(result.stdout, /^LOOMARR_TAMAGUI_COMPILER=0$/m);
  } finally {
    rmSync(bin, { force: true, recursive: true });
  }
});

test("refuses a native build before handoff when available memory is below the configured floor", () => {
  const result = spawnSync("bash", [path.join(__dirname, "build-android-client.sh"), "tv"], {
    encoding: "utf8",
    env: {
      ANDROID_HOME: "/android-sdk",
      LOOMARR_ANDROID_MIN_AVAILABLE_KB: "999999999",
      PATH: process.env.PATH,
    },
  });

  assert.equal(result.status, 1);
  assert.match(result.stderr, /refusing native build:/);
});

test("invalidates Metro transforms that contain compile-time public configuration", () => {
  const script = readFileSync(path.join(__dirname, "build-android-client.sh"), "utf8");

  assert.match(script, /expo export:embed[\s\S]*--reset-cache/);
});

test("keeps the P5 Guide benchmark inside the same resource limits and rejects non-Shield devices", () => {
  const script = readFileSync(path.join(__dirname, "build-android-client.sh"), "utf8");

  assert.match(script, /:macrobenchmark:connectedBenchmarkAndroidTest/);
  assert.match(script, /ANDROID_SERIAL must identify the physical Shield/);
  assert.match(script, /device_model[\s\S]*SHIELD/);
  assert.match(script, /verify-tv-macrobenchmark\.mjs/);
  assert.match(script, /ACTION[\s\S]*--inside-memory-scope/);
  assert.match(script, /macrobenchmark[\s\S]*gradle_node_env="production"/);
});
