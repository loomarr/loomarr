import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { createSocket } from "node:dgram";
import { once } from "node:events";
import { mkdtempSync, readFileSync, rmSync } from "node:fs";
import { createServer } from "node:net";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { describe, it } from "node:test";
import { fileURLToPath } from "node:url";

const journey = readFileSync(new URL("test-tv-emulator-journey.sh", import.meta.url), "utf8");
const server = readFileSync(new URL("tv-emulator-fixture-server.mjs", import.meta.url), "utf8");
const serverPath = fileURLToPath(new URL("tv-emulator-fixture-server.mjs", import.meta.url));

const listen = (server, port, host) =>
  new Promise((resolve, reject) => {
    server.once("error", reject);
    server.listen(port, host, resolve);
  });

const bind = (socket, port, host) =>
  new Promise((resolve, reject) => {
    socket.once("error", reject);
    socket.bind(port, host, resolve);
  });

describe("TV emulator journey", () => {
  it("builds the production TV root for the connected emulator and starts with fresh storage", () => {
    assert.match(journey, /EXPO_PUBLIC_LOOMARR_URL=/);
    assert.match(journey, /getprop ro\.product\.cpu\.abi/);
    assert.match(journey, /arm64-v8a\|x86_64/);
    assert.match(journey, /LOOMARR_ANDROID_ARCHITECTURES="\$\{emulator_architecture\}"/);
    assert.match(journey, /build-android-client\.sh" tv/);
    assert.match(
      readFileSync(new URL("build-android-client.sh", import.meta.url), "utf8"),
      /expo export:embed[\s\S]*--reset-cache/,
    );
    assert.match(journey, /install -r/);
    assert.match(journey, /shell pm clear/);
    assert.match(journey, /shell am start -W -n "\$\{PACKAGE_ID\}\/\.MainActivity"/);
  });

  it("drives every item 33 remote and lifecycle checkpoint", () => {
    for (const checkpoint of [
      "fresh pairing code",
      "Watching home",
      "Guide",
      "Watching after Back",
      "Surf",
      "Surf tune",
      "number entry",
      "number tune",
      "Watching after number tune",
      "background event-stream release",
      "foreground catalog refresh",
      "foreground retune",
      "disconnect confirmation",
      "device revocation",
    ]) {
      assert.match(journey, new RegExp(checkpoint));
    }
    assert.match(journey, /key KEYCODE_DPAD_CENTER/);
    assert.match(journey, /key KEYCODE_BACK/);
    assert.match(journey, /key KEYCODE_DPAD_LEFT/);
    assert.equal(journey.match(/key KEYCODE_7/g)?.length, 2);
    assert.match(server, /Classic Animation", 77/);
    assert.match(journey, /key KEYCODE_HOME/);
  });

  it("asserts rendered UI and authenticated server effects rather than timing alone", () => {
    assert.match(journey, /uiautomator dump/);
    assert.match(journey, /wait_for_ui/);
    assert.match(journey, /wait_for_state/);
    assert.match(server, /request\.headers\.authorization === `Bearer \$\{deviceToken\}`/);
    assert.match(server, /state\.playUrlChannels\.push\(channelId\)/);
    assert.match(server, /state\.eventDisconnects \+= 1/);
    assert.match(server, /state\.revocations \+= 1/);
  });

  it("starts the manual-entry fixture while the production discovery port is occupied", async () => {
    const discoveryBlocker = createSocket("udp4");
    const portReservation = createServer();
    const mediaDirectory = mkdtempSync(join(tmpdir(), "loomarr-tv-manual-fixture-"));
    let fixture;

    try {
      await bind(discoveryBlocker, 0, "127.0.0.1");
      const discoveryAddress = discoveryBlocker.address();
      assert(discoveryAddress && typeof discoveryAddress === "object");
      await listen(portReservation, 0, "127.0.0.1");
      const address = portReservation.address();
      assert(address && typeof address === "object");
      const fixturePort = address.port;
      await new Promise((resolve, reject) =>
        portReservation.close((error) => (error ? reject(error) : resolve())),
      );

      fixture = spawn(
        process.execPath,
        [
          serverPath,
          String(fixturePort),
          mediaDirectory,
          "127.0.0.1",
          "disabled",
          String(discoveryAddress.port),
        ],
        {
          stdio: ["ignore", "pipe", "pipe"],
        },
      );
      let fixtureOutput = "";
      fixture.stdout.on("data", (chunk) => {
        fixtureOutput += chunk;
      });
      fixture.stderr.on("data", (chunk) => {
        fixtureOutput += chunk;
      });

      let ready = false;
      for (let attempt = 0; attempt < 40; attempt += 1) {
        if (fixture.exitCode !== null) break;
        try {
          const response = await fetch(`http://127.0.0.1:${fixturePort}/__journey`);
          ready = response.ok;
        } catch {
          // The fixture may still be starting.
        }
        if (ready) break;
        await new Promise((resolve) => setTimeout(resolve, 25));
      }
      assert.equal(ready, true, fixtureOutput || "manual-entry fixture exited before HTTP became ready");
      await new Promise((resolve) => setTimeout(resolve, 100));
      assert.equal(
        fixture.exitCode,
        null,
        fixtureOutput || "manual-entry fixture exited after becoming ready",
      );
    } finally {
      if (fixture?.exitCode === null) {
        fixture.kill("SIGTERM");
        await once(fixture, "exit");
      }
      discoveryBlocker.close();
      if (portReservation.listening) portReservation.close();
      rmSync(mediaDirectory, { force: true, recursive: true });
    }
  });
});
