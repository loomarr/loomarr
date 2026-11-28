// Every port a device reads through parses only what it uses: a newer server's additions leave it
// working, and a body of another shape is refused instead of flowing on under the contract's type.

import { describe, expect, it } from "vitest";
import { createAccountSource } from "../account";
import { createChannelAudioSource } from "../channel-audio";
import { createChannelSubtitlesSource } from "../channel-subtitles";
import { createGuideSourcePort } from "../guide";
import { createMyChannelsPort } from "../my-channels";
import { createPairingTransport } from "../pairing";
import { createServerVersionSource } from "../system-version";

const signal = new AbortController().signal;
const serve = (body: (url: string) => unknown): typeof fetch =>
  (async (input: unknown) => new Response(JSON.stringify(body(String(input))))) as typeof fetch;
const newer = { addedLater: "unheard-of" };

const airing = { scheduleBlockId: "b1", startMs: 1, stopMs: 2, title: "Comet Cadet" };
const channel = { channelId: "c1", name: "Seven", number: 7, pendingCount: 0 };

interface Port {
  name: string;
  read: (request: typeof fetch) => Promise<unknown>;
  newerBody: (url: string) => unknown;
  expected: unknown;
  wrongBody: unknown;
}

const ports: Port[] = [
  {
    name: "guide",
    read: (request) => createGuideSourcePort(request).load({ fromMs: 0, toMs: 5 }, signal),
    newerBody: () => ({
      ...newer,
      channels: [
        { ...channel, ...newer, status: "hibernating", airings: [{ ...airing, ...newer, kind: "hologram" }] },
      ],
      fromMs: 0,
      toMs: 5,
    }),
    expected: {
      channels: [{ ...channel, status: "hibernating", airings: [{ ...airing, kind: "hologram" }] }],
      fromMs: 0,
      toMs: 5,
    },
    wrongBody: { channels: [{ channelId: "c1" }], fromMs: 0, toMs: 5 },
  },
  {
    name: "account",
    read: (request) => createAccountSource(request).load(signal),
    newerBody: () => ({ ...newer, name: "Sam", role: "guest" }),
    expected: { name: "Sam", role: "guest" },
    wrongBody: { role: "admin" },
  },
  {
    name: "my channels",
    read: (request) => createMyChannelsPort(request).load(signal),
    newerBody: () => ({ ...newer, favourites: [{ ...newer, channelId: "c1" }], recent: [] }),
    expected: { favourites: [{ channelId: "c1" }], recent: [] },
    wrongBody: { favourites: null, recent: [] },
  },
  {
    name: "channel subtitles",
    read: (request) => createChannelSubtitlesSource(request).load("c1", { from: 0, to: 1 }, signal),
    newerBody: () => ({ ...newer, cues: [], pending: false, tracks: [] }),
    expected: { cues: [], pending: false, tracks: [] },
    wrongBody: { pending: "no" },
  },
  {
    name: "channel audio",
    read: (request) => createChannelAudioSource(request).load("c1", signal),
    newerBody: (url) =>
      url.endsWith("/tracks")
        ? { ...newer, audio: [] }
        : { ...newer, status: "hibernating", policy: { ...newer, playout: { audioLanguage: "spa" } } },
    expected: expect.any(Array),
    wrongBody: { policy: "none" },
  },
  {
    name: "server version",
    read: (request) => createServerVersionSource(request).load(signal),
    newerBody: () => ({ ...newer, version: "v1", dirty: false }),
    expected: "v1",
    wrongBody: { version: 1 },
  },
  {
    name: "pairing",
    read: async (request) =>
      (await createPairingTransport("http://tv.test", request).start("TV", signal)).body,
    newerBody: () => ({ ...newer, deviceCode: "d", expiresAt: "later", interval: 5, userCode: "ABCD" }),
    expected: { deviceCode: "d", expiresAt: "later", interval: 5, userCode: "ABCD" },
    wrongBody: { userCode: "ABCD" },
  },
];

describe.each(ports)("the $name read", ({ read, newerBody, expected, wrongBody }) => {
  it("still parses what a newer server sends", async () => {
    await expect(read(serve(newerBody))).resolves.toEqual(expected);
  });

  it("refuses a body of another shape", async () => {
    await expect(read(serve(() => wrongBody))).rejects.toMatchObject({ name: "ContractError" });
  });
});
