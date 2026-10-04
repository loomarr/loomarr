import { describe, expect, it } from "vitest";

import {
  createTvAutoTuneSetting,
  DEFAULT_NUMBER_ENTRY_MS,
  initialTvWatchingRemoteState,
  MAX_NUMBER_ENTRY_MS,
  reduceTvWatchingRemote,
  tvNumberEntryPresentation,
} from "../index";

const memoryStorage = (initial: Record<string, string> = {}) => {
  const data = { ...initial };
  return {
    getItem: async (key: string) => data[key] ?? null,
    setItem: async (key: string, value: string) => {
      data[key] = value;
    },
  };
};

describe("TV auto-tune duration setting", () => {
  it("defaults to the standard countdown when nothing is stored", async () => {
    const setting = createTvAutoTuneSetting(memoryStorage());
    expect(await setting.load()).toBe(DEFAULT_NUMBER_ENTRY_MS);
  });

  it("round-trips a saved duration", async () => {
    const storage = memoryStorage();
    const setting = createTvAutoTuneSetting(storage);
    expect(await setting.save(4_000)).toBe(4_000);
    expect(await setting.load()).toBe(4_000);
  });

  it("clamps a saved value to the supported range (WCAG 2.2.1, #1659 decision N4)", async () => {
    const setting = createTvAutoTuneSetting(memoryStorage());
    expect(await setting.save(60_000)).toBe(MAX_NUMBER_ENTRY_MS);
    expect(await setting.save(-5)).toBe(1_200);
  });

  it("times the remote's digit entry and its countdown by the stored duration, as app.tsx wires it", async () => {
    const durationMs = await createTvAutoTuneSetting(
      memoryStorage({ "loomarr.tv.autoTuneDurationMs": "5000" }),
    ).load();
    const channels = [{ name: "Nature Documentaries", number: 21 }];
    let state = reduceTvWatchingRemote(
      initialTvWatchingRemoteState,
      { atMs: 0, digit: "2", key: "digit" },
      durationMs,
    ).state;
    state = reduceTvWatchingRemote(state, { atMs: 100, digit: "1", key: "digit" }, durationMs).state;

    expect(tvNumberEntryPresentation(state, channels)).toEqual({
      channelName: "Nature Documentaries",
      digits: "21",
      expiresAtMs: 5_100,
    });
    // The default 1.2 s has passed, but the stored duration hasn't: no tune yet.
    expect(reduceTvWatchingRemote(state, { atMs: 1_300, key: "timeout" }, durationMs).intent).toBeUndefined();
    expect(reduceTvWatchingRemote(state, { atMs: 5_100, key: "timeout" }, durationMs).intent).toEqual({
      digits: "21",
      kind: "tune-number",
    });
  });

  it("falls back to the default when the stored value is corrupt", async () => {
    const setting = createTvAutoTuneSetting(memoryStorage({ "loomarr.tv.autoTuneDurationMs": "nope" }));
    expect(await setting.load()).toBe(DEFAULT_NUMBER_ENTRY_MS);
  });
});
