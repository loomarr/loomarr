import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { createSocket } from "node:dgram";
import { once } from "node:events";
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { createServer } from "node:net";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { describe, it } from "node:test";
import { fileURLToPath } from "node:url";

const journey = readFileSync(new URL("test-tv-emulator-journey.sh", import.meta.url), "utf8");
const serverPath = fileURLToPath(new URL("tv-emulator-fixture-server.mjs", import.meta.url));

const listen = (server, port, host) =>
  new Promise((resolve, reject) => {
    server.once("error", reject);
    server.listen(port, host, resolve);
  });

// startFixture runs the journey's fixture server on a free port, discovery off, and returns its
// base URL with a stop function that also removes its media directory.
const startFixture = async () => {
  const reservation = createServer();
  const mediaDirectory = mkdtempSync(join(tmpdir(), "loomarr-tv-fixture-"));
  await listen(reservation, 0, "127.0.0.1");
  const { port } = reservation.address();
  await new Promise((resolve) => reservation.close(resolve));
  const fixture = spawn(
    process.execPath,
    [serverPath, String(port), mediaDirectory, "127.0.0.1", "disabled"],
    {
      stdio: "ignore",
    },
  );
  const base = `http://127.0.0.1:${port}`;
  for (let attempt = 0; attempt < 40; attempt += 1) {
    const ready = await fetch(`${base}/__journey`).then(
      (response) => response.ok,
      () => false,
    );
    if (ready) break;
    await new Promise((resolve) => setTimeout(resolve, 25));
  }
  const stop = async () => {
    if (fixture.exitCode === null) {
      fixture.kill("SIGTERM");
      await once(fixture, "exit");
    }
    rmSync(mediaDirectory, { force: true, recursive: true });
  };
  return { base, mediaDirectory, stop };
};

const journeyState = async (base) => (await fetch(`${base}/__journey`)).json();

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
      "◀ at now does not page before now",
      "▲ from the top row reaches the filters",
      "▶ off a whole-window programme pages forward",
      "◀ inside the paged window does not page",
      "◀ after paging forward pages back to now",
      "the channel-number wait row",
      "the 5 s choice",
      "the row showing the saved 5 s",
      "the 5 s wait tuning the typed Channel",
      "the stored 5 s after relaunch",
    ]) {
      assert.match(journey, new RegExp(checkpoint));
    }
    assert.match(journey, /key KEYCODE_DPAD_CENTER/);
    assert.match(journey, /key KEYCODE_BACK/);
    assert.match(journey, /key KEYCODE_DPAD_LEFT/);
    assert.equal(journey.match(/key KEYCODE_7/g)?.length, 2);
    assert.match(journey, /key KEYCODE_HOME/);
  });

  it("asserts rendered UI and server effects rather than timing alone", () => {
    assert.match(journey, /uiautomator dump/);
    assert.match(journey, /wait_for_ui/);
    assert.match(journey, /wait_for_state/);
  });

  // #1888: Watching warms the neighbouring Channel with the same play-url request a tune makes, and a
  // tune can reuse a warmed source without minting again, so the last play-url names the neighbour.
  it("judges a tune by the Channel whose stream the player opened, not the last play-url", () => {
    assert.doesNotMatch(journey, /playUrlChannels/);
    assert.match(journey, /\.tunedChannels\[-1\] == "science-fiction"/);
    assert.match(journey, /\.tunedChannels\[-1\] == "classic-animation"/);
  });

  it("records a tune only for a stream the player opens, never for a neighbour warm", async () => {
    const { base, mediaDirectory, stop } = await startFixture();
    try {
      writeFileSync(join(mediaDirectory, "media.m3u8"), "#EXTM3U\n#EXT-X-ENDLIST\n");
      writeFileSync(join(mediaDirectory, "media0.ts"), "segment");
      const headers = { authorization: "Bearer journey-device-token" };
      const mint = async (channelId) =>
        (await fetch(`${base}/v1/channels/${channelId}/play-url`, { method: "POST", headers })).json();

      const tuned = await mint("classic-animation");
      const neighbour = await mint("science-fiction");
      assert.equal(tuned.relativeUrl, "/journey/classic-animation/media.m3u8");
      assert.equal(neighbour.relativeUrl, "/journey/science-fiction/media.m3u8");

      assert.equal((await fetch(`${base}${neighbour.relativeUrl}?mode=warm`)).status, 200);
      assert.equal((await fetch(`${base}/journey/science-fiction/media0.ts`)).status, 200);
      assert.equal((await fetch(`${base}${tuned.relativeUrl}`)).status, 200);
      assert.equal((await fetch(`${base}/journey/no-such-channel/media.m3u8`)).status, 404);

      const state = await journeyState(base);
      assert.deepEqual(state.playUrlChannels, ["classic-animation", "science-fiction"]);
      assert.deepEqual(state.tunedChannels, ["classic-animation"]);
    } finally {
      await stop();
    }
  });

  // #1888: the journey runs for minutes on the wall clock the Guide reads, so a break a few minutes
  // after the first Guide read moved into the live window partway through and broke later checkpoints.
  it("keeps the live window's break well ahead of the whole journey, and the filler phase inside it", async () => {
    const { base, stop } = await startFixture();
    try {
      const headers = { authorization: "Bearer journey-device-token" };
      const live = await (await fetch(`${base}/v1/guide`, { headers })).json();
      const { guideEpochMs } = await journeyState(base);
      const commercial = live.channels[0].airings.find(({ kind }) => kind === "filler");
      assert.ok(commercial.startMs - guideEpochMs >= 30 * 60_000);
      assert.ok(commercial.stopMs - commercial.startMs >= 60_000);

      await fetch(`${base}/__journey/phase/filler`, { method: "POST" });
      const filler = await (
        await fetch(`${base}/v1/channels/classic-animation/play-url`, { method: "POST", headers })
      ).json();
      assert.ok(filler.serverTimeMs > commercial.startMs && filler.serverTimeMs < commercial.stopMs);
      await fetch(`${base}/__journey/phase/programme-after`, { method: "POST" });
      const after = await (
        await fetch(`${base}/v1/channels/classic-animation/play-url`, { method: "POST", headers })
      ).json();
      assert.ok(after.serverTimeMs > commercial.stopMs);
    } finally {
      await stop();
    }
  });

  // The journey's number entry types 7 7 and waits on the fixture's recorded effects, so the
  // fixture must serve channel 77 and record each effect only for the paired device's token.
  it("records the authenticated effects the journey waits on", async () => {
    const { base, stop } = await startFixture();
    try {
      const headers = { authorization: "Bearer journey-device-token" };
      assert.equal((await fetch(`${base}/v1/channels`)).status, 401);
      assert.equal((await fetch(`${base}/v1/auth/device`, { method: "DELETE" })).status, 401);

      const { channels } = await (await fetch(`${base}/v1/channels`, { headers })).json();
      assert.ok(channels.some(({ name, number }) => name === "Classic Animation" && number === 77));

      await fetch(`${base}/v1/channels/classic-animation/play-url`, { method: "POST", headers });
      const events = new AbortController();
      const stream = await fetch(`${base}/v1/events`, { headers, signal: events.signal });
      assert.equal(stream.status, 200);
      events.abort();
      assert.equal((await fetch(`${base}/v1/auth/device`, { method: "DELETE", headers })).status, 204);

      let state = await journeyState(base);
      for (let attempt = 0; attempt < 40 && state.eventDisconnects === 0; attempt += 1) {
        await new Promise((resolve) => setTimeout(resolve, 25));
        state = await journeyState(base);
      }
      assert.deepEqual(state.playUrlChannels, ["classic-animation"]);
      assert.equal(state.eventDisconnects, 1);
      assert.equal(state.revocations, 1);
    } finally {
      await stop();
    }
  });

  it("serves the Guide window the app asks for, so paging is observable (#1659 decision N4)", async () => {
    const { base, stop } = await startFixture();
    try {
      const headers = { authorization: "Bearer journey-device-token" };
      const live = await (await fetch(`${base}/v1/guide`, { headers })).json();
      const from = live.toMs;
      const to = from + (live.toMs - live.fromMs);
      const paged = await (await fetch(`${base}/v1/guide?from=${from}&to=${to}`, { headers })).json();

      assert.deepEqual([paged.fromMs, paged.toMs], [from, to]);
      const [classic, science] = paged.channels;
      assert.deepEqual(
        classic.airings.map(({ scheduleBlockId }) => scheduleBlockId),
        ["later-first", "later-second"],
      );
      assert.deepEqual([science.airings[0].startMs, science.airings[0].stopMs], [from, to]);
      const state = await journeyState(base);
      assert.deepEqual(state.guideWindows.at(-1), { fromMs: from, toMs: to });
    } finally {
      await stop();
    }
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
