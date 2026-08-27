#!/usr/bin/env node
import { execFileSync } from "node:child_process";
import { pathToFileURL } from "node:url";

const PACKAGE_NAME = "media.loomarr.tv.prototype";
const ACTIVITY_NAME = `${PACKAGE_NAME}/.MainActivity`;
const WAIT_MS = 10_000;

const surfaceFromXml = (xml) => {
  if (xml.includes("PAIRING CODE")) return "pairing";
  if (xml.includes('content-desc="Programme guide"')) return "guide";
  if (xml.includes('content-desc="Channel surfer"')) return "surf";
  if (xml.includes('content-desc="Show playback controls"')) return "watching";
  return "unknown";
};

const assertSurface = (xml, expected) => {
  const actual = surfaceFromXml(xml);
  if (actual !== expected) throw new Error(`expected ${expected} surface; found ${actual}`);
  if ((expected === "guide" || expected === "surf") && xml.includes("Show playback controls")) {
    throw new Error(`Watching chrome remained accessible behind ${expected}`);
  }
};

const createAdb = (serial) => {
  const run = (...args) =>
    execFileSync("adb", ["-s", serial, ...args], {
      encoding: "utf8",
      stdio: ["ignore", "pipe", "pipe"],
    });
  return {
    dump: () => {
      run("shell", "uiautomator", "dump", "/sdcard/loomarr-acceptance.xml");
      return run("exec-out", "cat", "/sdcard/loomarr-acceptance.xml");
    },
    key: (name) => run("shell", "input", "keyevent", name),
    model: () => run("shell", "getprop", "ro.product.model").trim(),
    qemu: () => run("shell", "getprop", "ro.kernel.qemu").trim(),
    start: () => run("shell", "am", "start", "-W", "-n", ACTIVITY_NAME),
  };
};

const waitForSurface = async (adb, expected, timeoutMs = WAIT_MS) => {
  const deadline = Date.now() + timeoutMs;
  let lastXml = "";
  while (Date.now() < deadline) {
    lastXml = adb.dump();
    if (surfaceFromXml(lastXml) === expected) {
      assertSurface(lastXml, expected);
      return lastXml;
    }
    await new Promise((resolve) => setTimeout(resolve, 250));
  }
  throw new Error(`timed out waiting for ${expected}; last surface was ${surfaceFromXml(lastXml)}`);
};

const verifyTvEmulatorJourney = async (adb) => {
  if (adb.qemu() !== "1") throw new Error(`refusing ${adb.model() || "device"}; this gate is emulator-only`);
  const initial = adb.dump();
  if (surfaceFromXml(initial) === "pairing") {
    throw new Error("emulator is not paired; approve its code before running the journey gate");
  }
  assertSurface(initial, "watching");

  adb.key("KEYCODE_DPAD_CENTER");
  await waitForSurface(adb, "guide");
  adb.key("KEYCODE_BACK");
  await waitForSurface(adb, "watching");

  adb.key("KEYCODE_DPAD_LEFT");
  await waitForSurface(adb, "surf");
  adb.key("KEYCODE_BACK");
  await waitForSurface(adb, "watching");

  adb.key("KEYCODE_HOME");
  adb.start();
  await waitForSurface(adb, "watching");
  return { model: adb.model() };
};

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  const serial = process.argv[2];
  if (!serial) {
    console.error("usage: verify-tv-emulator-journey.mjs <emulator-serial>");
    process.exit(2);
  }
  try {
    const result = await verifyTvEmulatorJourney(createAdb(serial));
    console.log(`Android TV emulator journey passed on ${result.model}`);
  } catch (error) {
    console.error(`Android TV emulator journey failed: ${error.message}`);
    process.exit(1);
  }
}

export { assertSurface, surfaceFromXml, verifyTvEmulatorJourney };
