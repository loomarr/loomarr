import assert from "node:assert/strict";
import { spawn, spawnSync } from "node:child_process";
import { mkdtempSync, readdirSync, readFileSync, rmSync } from "node:fs";
import os from "node:os";
import path from "node:path";
import { test } from "node:test";
import { fileURLToPath } from "node:url";
import { cgroupDirectory, memoryEvents, memorySnapshot } from "./profile-android-build.mjs";

const script = fileURLToPath(new URL("./profile-android-build.mjs", import.meta.url));
const mount = "30 20 0:28 / /sys/fs/cgroup rw - cgroup2 cgroup rw";

test("resolves the actual mounted cgroup scope, including a subtree mount", () => {
  assert.equal(cgroupDirectory("0::/runner/build", mount), "/sys/fs/cgroup/runner/build");
  assert.equal(
    cgroupDirectory("0::/runner/build", mount.replace(" / /sys/", " /runner /sys/")),
    "/sys/fs/cgroup/build",
  );
  assert.equal(cgroupDirectory("0::/outside", mount.replace(" / /sys/", " /runner /sys/")), undefined);
  assert.equal(cgroupDirectory("0::/../outside", mount), undefined);
  assert.equal(cgroupDirectory("5:memory:/legacy", mount), undefined);
});

test("missing memory evidence remains unavailable instead of becoming zero", () => {
  const files = {
    "/proc/self/cgroup": "0::/runner",
    "/proc/self/mountinfo": mount,
    "/proc/meminfo": "MemAvailable: 1234 kB",
    "/sys/fs/cgroup/runner/memory.current": "100",
    "/sys/fs/cgroup/runner/memory.peak": "900",
    "/sys/fs/cgroup/runner/memory.max": "max",
    "/sys/fs/cgroup/runner/memory.events": "oom 0\noom_kill 0\nmax 2",
  };
  const reader = (name) => {
    if (!(name in files)) throw new Error("missing");
    return files[name];
  };
  const observed = memorySnapshot(reader);
  assert.equal(observed.currentBytes, 100);
  assert.equal(observed.lifetimePeakBytes, 900);
  assert.equal(observed.limitBytes, null);
  assert.equal(observed.hostAvailableBytes, 1234 * 1024);
  assert.deepEqual(observed.events, { oom: 0, oom_kill: 0, max: 2 });
  delete files["/sys/fs/cgroup/runner/memory.peak"];
  assert.equal(memorySnapshot(reader).status, "unavailable");
  assert.throws(() => memoryEvents("oom 0\nmax 0"), /incomplete/);
});

const temporary = (t) => {
  const dir = mkdtempSync(path.join(os.tmpdir(), "android-profile-test-"));
  t.after(() => rmSync(dir, { recursive: true, force: true }));
  return dir;
};

const report = (dir) => {
  const runs = readdirSync(dir).filter((name) => name.startsWith("run-"));
  assert.equal(runs.length, 1);
  return JSON.parse(readFileSync(path.join(dir, runs[0], "report.json"), "utf8"));
};

for (const code of [0, 7]) {
  test(`preserves build exit ${code} and retains observations without environment secrets`, (t) => {
    const dir = temporary(t);
    const result = spawnSync(
      process.execPath,
      [script, dir, "--", process.execPath, "-e", `process.exit(${code})`],
      {
        encoding: "utf8",
        env: { ...process.env, PROFILE_TEST_SECRET: "must-not-be-recorded" },
      },
    );
    assert.equal(result.status, code, result.stderr);
    const data = report(dir);
    assert.equal(data.outcome.code, code);
    assert.ok(data.samples >= 2);
    assert.ok(data.durationMs >= 0);
    assert.ok(!JSON.stringify(data).includes("must-not-be-recorded"));
  });
}

test("a command that cannot start cannot produce a successful profile", (t) => {
  const dir = temporary(t);
  const result = spawnSync(process.execPath, [script, dir, "--", path.join(dir, "absent-command")], {
    encoding: "utf8",
  });
  assert.equal(result.status, 1);
  assert.equal(report(dir).outcome.spawnError, "ENOENT");
});

test("cancellation reaches the build process group and retains its failed outcome", {
  timeout: 10000,
}, async (t) => {
  const dir = mkdtempSync(path.join(os.tmpdir(), "android-profile-cancel-test-"));
  const child = spawn(
    process.execPath,
    [
      script,
      dir,
      "--",
      process.execPath,
      "-e",
      'const {spawn} = require("node:child_process"); const fs = require("node:fs"); const grandchild = spawn(process.execPath, ["-e", "setInterval(() => {}, 1000)"], {stdio: "inherit"}); fs.writeFileSync(process.argv[1], String(process.pid)); process.stdout.write("ready\\n"); setInterval(() => {}, 1000)',
      path.join(dir, "grandchild.pid"),
    ],
    { stdio: ["ignore", "pipe", "pipe"] },
  );
  t.after(() => {
    try {
      child.kill("SIGKILL");
    } catch {}
    try {
      process.kill(-Number(readFileSync(path.join(dir, "grandchild.pid"), "utf8")), "SIGKILL");
    } catch {}
    rmSync(dir, { recursive: true, force: true });
  });
  await new Promise((resolve, reject) => {
    child.stdout.once("data", resolve);
    child.once("error", reject);
    child.once("exit", () => reject(new Error("profiler exited before the build started")));
  });
  const closed = new Promise((resolve) => child.once("close", (code) => resolve(code)));
  child.kill("SIGTERM");
  assert.equal(await closed, 143);
  assert.equal(report(dir).outcome.signal, "SIGTERM");
});
