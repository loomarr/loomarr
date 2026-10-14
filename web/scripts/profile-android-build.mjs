import { spawn } from "node:child_process";
import { appendFileSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

const read = (file) => readFileSync(file, "utf8").trim();
const unescapeMount = (value) =>
  value.replace(/\\([0-7]{3})/g, (_, octal) => String.fromCharCode(Number.parseInt(octal, 8)));

const cgroupDirectory = (membership, mountinfo) => {
  const member = membership
    .split("\n")
    .find((line) => line.startsWith("0::"))
    ?.slice(3);
  if (!member?.startsWith("/") || member.split("/").includes("..")) return undefined;
  for (const line of mountinfo.split("\n")) {
    const [left, right] = line.split(" - ");
    if (!right?.startsWith("cgroup2 ")) continue;
    const fields = left.split(" ");
    const root = unescapeMount(fields[3] ?? "");
    const mount = unescapeMount(fields[4] ?? "");
    if (!mount.startsWith("/")) continue;
    if (member === root) return mount;
    const prefix = root === "/" ? "/" : `${root}/`;
    if (member.startsWith(prefix)) return path.join(mount, member.slice(prefix.length));
  }
  return undefined;
};

const memoryEvents = (text) => {
  const events = {};
  for (const line of text.split("\n")) {
    const [key, value] = line.trim().split(/\s+/);
    if (["oom", "oom_kill", "max"].includes(key) && /^\d+$/.test(value ?? "")) events[key] = Number(value);
  }
  if (!["oom", "oom_kill", "max"].every((key) => Number.isSafeInteger(events[key])))
    throw new Error("incomplete memory event counters");
  return events;
};

const bytes = (text) => {
  if (!/^\d+$/.test(text) || !Number.isSafeInteger(Number(text)))
    throw new Error("invalid memory byte count");
  return Number(text);
};

const memorySnapshot = (reader = read) => {
  let hostAvailableBytes;
  try {
    const available = reader("/proc/meminfo").match(/^MemAvailable:\s+(\d+) kB$/m);
    if (available) hostAvailableBytes = bytes(available[1]) * 1024;
  } catch {
    /* Non-Linux hosts do not provide this observation. */
  }
  try {
    const directory = cgroupDirectory(reader("/proc/self/cgroup"), reader("/proc/self/mountinfo"));
    if (!directory) throw new Error("cgroup v2 mount cannot be resolved");
    const limit = reader(path.join(directory, "memory.max"));
    return {
      status: "available",
      directory,
      hostAvailableBytes,
      currentBytes: bytes(reader(path.join(directory, "memory.current"))),
      lifetimePeakBytes: bytes(reader(path.join(directory, "memory.peak"))),
      limitBytes: limit === "max" ? null : bytes(limit),
      events: memoryEvents(reader(path.join(directory, "memory.events"))),
    };
  } catch {
    return { status: "unavailable", reason: "cgroup_v2_memory_observation_unavailable", hostAvailableBytes };
  }
};

const profileBuild = async (output, command, args) => {
  if (process.platform === "win32") throw new Error("Android profiling requires POSIX process groups");
  const started = new Date();
  const start = performance.now();
  const directory = path.resolve(output, `run-${started.toISOString().replaceAll(":", "-")}-${process.pid}`);
  mkdirSync(directory, { recursive: true, mode: 0o700 });
  const before = memorySnapshot();
  const report = {
    version: 1,
    startedAt: started.toISOString(),
    source: {
      // biome-ignore lint/suspicious/noUndeclaredEnvVars: Uncached Make profiling observes CI provenance outside Turbo tasks.
      sha: process.env.GITHUB_SHA ?? null,
      // biome-ignore lint/suspicious/noUndeclaredEnvVars: Uncached Make profiling observes CI provenance outside Turbo tasks.
      runID: process.env.GITHUB_RUN_ID ?? null,
      // biome-ignore lint/suspicious/noUndeclaredEnvVars: Uncached Make profiling observes CI provenance outside Turbo tasks.
      attempt: process.env.GITHUB_RUN_ATTEMPT ?? null,
    },
    runner: {
      platform: os.platform(),
      release: os.release(),
      architecture: os.arch(),
      cpuModel: os.cpus()[0]?.model,
      logicalCPUs: os.cpus().length,
      availableParallelism: os.availableParallelism(),
      totalMemoryBytes: os.totalmem(),
      node: process.version,
      // biome-ignore lint/suspicious/noUndeclaredEnvVars: Uncached Make profiling observes CI provenance outside Turbo tasks.
      imageOS: process.env.ImageOS ?? null,
      // biome-ignore lint/suspicious/noUndeclaredEnvVars: Uncached Make profiling observes CI provenance outside Turbo tasks.
      imageVersion: process.env.ImageVersion ?? null,
    },
    before,
    sampleIntervalMs: 250,
    samples: 0,
    unavailableSamples: 0,
    sampledPeakCurrentBytes: null,
    minimumHostAvailableBytes: null,
  };
  const child = spawn(command, args, {
    stdio: "inherit",
    detached: true,
    env: { ...process.env, ANDROID_BUILD_PROFILE_DIR: directory },
  });
  const forward = (signal) => {
    if (!child.pid) return;
    try {
      process.kill(-child.pid, signal);
    } catch (error) {
      if (error.code !== "ESRCH") throw error;
    }
  };
  const interrupt = () => forward("SIGINT");
  const terminate = () => forward("SIGTERM");
  process.on("SIGINT", interrupt);
  process.on("SIGTERM", terminate);
  let profileError;
  const sample = () => {
    try {
      const value = memorySnapshot();
      report.samples++;
      if (value.status !== "available") report.unavailableSamples++;
      if (value.currentBytes !== undefined)
        report.sampledPeakCurrentBytes = Math.max(report.sampledPeakCurrentBytes ?? 0, value.currentBytes);
      if (value.hostAvailableBytes !== undefined)
        report.minimumHostAvailableBytes = Math.min(
          report.minimumHostAvailableBytes ?? Infinity,
          value.hostAvailableBytes,
        );
      appendFileSync(
        path.join(directory, "memory.ndjson"),
        `${JSON.stringify({ elapsedMs: Math.round(performance.now() - start), ...value })}\n`,
        { mode: 0o600 },
      );
    } catch (error) {
      profileError = error.message;
      terminate();
    }
  };
  const result = new Promise((resolve) => {
    child.once("error", (error) => resolve({ code: 1, spawnError: error.code ?? "spawn_failed" }));
    child.once("close", (code, signal) => resolve({ code, signal }));
  });
  sample();
  const timer = setInterval(sample, report.sampleIntervalMs);
  const outcome = await result;
  clearInterval(timer);
  sample();
  process.off("SIGINT", interrupt);
  process.off("SIGTERM", terminate);
  report.finishedAt = new Date().toISOString();
  report.durationMs = Math.round(performance.now() - start);
  report.after = memorySnapshot();
  report.outcome = outcome;
  report.profileError = profileError ?? null;
  report.scopeNote =
    "Inherited cgroup scope, possibly shared with runner processes. Lifetime peaks are not reset phase peaks; sampled current usage can miss spikes. No memory-safety qualification is implied.";
  if (
    before.status === "available" &&
    report.after.status === "available" &&
    before.directory === report.after.directory
  ) {
    report.memoryEventDeltas = Object.fromEntries(
      Object.keys(before.events).map((key) => [key, report.after.events[key] - before.events[key]]),
    );
  }
  writeFileSync(path.join(directory, "report.json"), `${JSON.stringify(report, null, 2)}\n`, { mode: 0o600 });
  process.stdout.write(`Android build profile: ${directory}/report.json\n`);
  if (profileError) return 1;
  return outcome.code ?? 128 + (os.constants.signals[outcome.signal] ?? 1);
};

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const [output, separator, command, ...args] = process.argv.slice(2);
  if (!output || separator !== "--" || !command) {
    process.stderr.write("usage: profile-android-build.mjs OUTPUT -- COMMAND [ARG...]\n");
    process.exitCode = 2;
  } else {
    try {
      process.exitCode = await profileBuild(output, command, args);
    } catch (error) {
      process.stderr.write(`${error.message}\n`);
      process.exitCode = 1;
    }
  }
}

export { cgroupDirectory, memoryEvents, memorySnapshot, profileBuild };
