import { CrashReporter } from "@loomarr/core/crash-reports";
import { afterEach, describe, expect, it, vi } from "vitest";

const native = vi.hoisted(() => ({
  listeners: new Map(),
  module: undefined,
}));

vi.mock("react-native", () => ({
  NativeEventEmitter: class {
    addListener(event, listener) {
      native.listeners.set(event, listener);
      return { remove: () => native.listeners.delete(event) };
    }
  },
  get NativeModules() {
    return { LoomarrCrashReports: native.module };
  },
}));

import { createNativeCrashSource, forwardNativeCrashes } from "../index.ts";

const device = {
  appBuild: "7",
  appVersion: "0.2.0",
  deviceModel: "iPhone17,1",
  osVersion: "27.0",
  platform: "ios",
};

const memoryStorage = () => {
  const files = new Map();
  return {
    files,
    storage: {
      list: () => [...files.keys()],
      read: (name) => files.get(name),
      remove: (name) => void files.delete(name),
      write: (name, contents) => void files.set(name, contents),
    },
  };
};

// What LoomarrCrashReports.swift hands JavaScript: MXDiagnosticPayload.jsonRepresentation() plus
// the window's end, as CrashPayloadStore persisted it.
const metricKitEnvelope = (id) => ({
  endedAt: "2026-10-13T11:59:00Z",
  id,
  payload: JSON.stringify({
    crashDiagnostics: [
      {
        callStackTree: {
          callStackPerThread: true,
          callStacks: [{ callStackRootFrames: [], threadAttributed: true }],
        },
        diagnosticMetaData: { appBuildVersion: "6", appVersion: "0.2.0", exceptionType: 1, signal: 11 },
        version: "1.0.0",
      },
    ],
    timeStampBegin: "2026-10-12 12:00:00",
    timeStampEnd: "2026-10-13 11:59:00",
  }),
});

const nativeModule = (envelopes) => ({
  addListener: vi.fn(),
  appBuild: "7",
  appVersion: "0.2.0",
  deviceModel: "iPhone17,1",
  osVersion: "",
  pending: vi.fn(async () => envelopes),
  remove: vi.fn(async (id) => {
    const index = envelopes.findIndex((envelope) => envelope.id === id);
    if (index >= 0) envelopes.splice(index, 1);
  }),
  removeListeners: vi.fn(),
  start: vi.fn(),
});

describe("native crash source", () => {
  afterEach(() => {
    native.module = undefined;
    native.listeners.clear();
  });

  it("is absent where MetricKit is not linked", () => {
    expect(createNativeCrashSource()).toBeUndefined();
  });

  it("subscribes to MetricKit, reads the build and device, and ignores malformed payloads", async () => {
    const valid = metricKitEnvelope("001-a");
    native.module = nativeModule([valid, { id: 3 }, null]);

    const source = createNativeCrashSource();
    const delivered = vi.fn();
    const unsubscribe = source?.subscribe(delivered);
    native.listeners.get("loomarrCrashPayloadsAvailable")?.();

    expect(native.module.start).toHaveBeenCalledTimes(1);
    expect(source?.device).toEqual({
      appBuild: "7",
      appVersion: "0.2.0",
      deviceModel: "iPhone17,1",
      osVersion: undefined,
    });
    expect(await source?.pending()).toEqual([valid]);
    expect(delivered).toHaveBeenCalledTimes(1);
    unsubscribe?.();
    expect(native.listeners.size).toBe(0);
  });
});

describe("forwarding MetricKit crashes", () => {
  it("queues each payload's crash as a native report, then removes it from native storage", async () => {
    native.module = nativeModule([metricKitEnvelope("001-a"), metricKitEnvelope("002-b")]);
    const source = createNativeCrashSource();
    const { storage } = memoryStorage();
    const reporter = new CrashReporter({ device, storage });

    expect(await forwardNativeCrashes(source, reporter)).toBe(2);

    expect(native.module.remove.mock.calls).toEqual([["001-a"], ["002-b"]]);
    expect(await source.pending()).toEqual([]);
    expect(reporter.pending()).toEqual([
      expect.objectContaining({
        appBuild: "6",
        kind: "native",
        message: "EXC_BAD_ACCESS (SIGSEGV)",
        nativePayload: expect.objectContaining({ timeStampEnd: "2026-10-13 11:59:00" }),
        occurredAt: "2026-10-13T11:59:00Z",
      }),
      expect.objectContaining({ kind: "native" }),
    ]);
  });

  it("leaves a payload in native storage when it cannot be queued", async () => {
    const envelopes = [metricKitEnvelope("001-a")];
    const source = { pending: async () => envelopes, remove: vi.fn() };
    const reporter = {
      captureNative: () => {
        throw new Error("disk full");
      },
    };

    await expect(forwardNativeCrashes(source, reporter)).rejects.toThrow("disk full");
    expect(source.remove).not.toHaveBeenCalled();
  });
});
