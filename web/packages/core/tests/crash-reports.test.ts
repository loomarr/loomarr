import type { ClientCrashReportInput } from "@loomarr/api/models/clientCrashReportInput";
import { describe, expect, it, vi } from "vitest";
import {
  BreadcrumbTrail,
  type CrashReportDevice,
  CrashReporter,
  type CrashReportSend,
  type CrashReportStorage,
  createBreadcrumbFetch,
  createCrashReportSender,
  type GlobalErrorUtils,
  installCrashHandler,
  scrubText,
  scrubUrl,
} from "../crash-reports";

const device: CrashReportDevice = {
  appBuild: "7",
  appVersion: "0.2.0",
  deviceModel: "iPhone17,1",
  osVersion: "27.0",
  platform: "ios",
};

const memoryStorage = () => {
  const files = new Map<string, string>();
  const storage: CrashReportStorage = {
    list: () => [...files.keys()],
    read: (name) => files.get(name),
    remove: (name) => void files.delete(name),
    write: (name, contents) => void files.set(name, contents),
  };
  return { files, storage };
};

const reporterAt = (storage: CrashReportStorage, start = Date.UTC(2026, 9, 13, 12)) => {
  let now = start;
  const reporter = new CrashReporter({ device, now: () => now++, storage });
  return reporter;
};

const utf8Bytes = (text: string) => new TextEncoder().encode(text).length;

describe("crash report scrubbing", () => {
  it("keeps only a URL's path", () => {
    expect(scrubUrl("https://tv.example.net:8443/v1/channels/7/play.m3u8?token=abc&exp=9#t=3")).toBe(
      "/v1/channels/7/play.m3u8",
    );
    expect(scrubUrl("http://192.168.1.20:8080?token=abc")).toBe("/");
    expect(scrubUrl("/v1/guide?from=1&token=abc")).toBe("/v1/guide");
  });

  it("reduces URLs in free text to paths and keeps a stack frame's position", () => {
    const text = [
      "Failed to load https://tv.example.net/v1/hls/7/index.m3u8?sig=s3cret&token=t0ken",
      "    at load (http://localhost:8081/index.bundle?platform=ios&dev=true:120:17)",
      "GET /v1/channels/7/play-url?quality=auto&token=abc failed",
    ].join("\n");

    const scrubbed = scrubText(text);

    expect(scrubbed).toBe(
      [
        "Failed to load /v1/hls/7/index.m3u8",
        "    at load (/index.bundle:120:17)",
        "GET /v1/channels/7/play-url failed",
      ].join("\n"),
    );
  });

  it("redacts credentials that are not inside a URL", () => {
    const scrubbed = scrubText(
      "Authorization: Bearer abc.def\ncookie=loomarr_session=xyz; other=1\nretry with Bearer zzz token=q1 ok",
    );

    expect(scrubbed).not.toMatch(/abc\.def|xyz|zzz|q1/);
    expect(scrubbed).toBe(
      "Authorization: [redacted]\ncookie=[redacted]\nretry with Bearer [redacted] token=[redacted] ok",
    );
  });
});

describe("breadcrumb trail", () => {
  it("keeps the last 50, scrubbed and cut to 256 bytes of UTF-8", () => {
    const trail = new BreadcrumbTrail(() => Date.UTC(2026, 9, 13));
    for (let index = 0; index < 60; index += 1) trail.add("navigation", `screen ${index}`);
    trail.add("log", `${"é".repeat(200)} https://host/x?token=1`);

    const crumbs = trail.snapshot();

    expect(crumbs).toHaveLength(50);
    expect(crumbs[0]?.message).toBe("screen 11");
    const last = crumbs.at(-1);
    expect(last).toMatchObject({ at: "2026-10-13T00:00:00.000Z", category: "log" });
    expect(utf8Bytes(last?.message ?? "")).toBeLessThanOrEqual(256);
    expect(last?.message).toBe("é".repeat(128));
  });

  it("records each request's method, path and status, never its query", async () => {
    const trail = new BreadcrumbTrail(() => 0);
    const fetcher = vi.fn<typeof fetch>(async () => new Response(null, { status: 204 }));
    const recorded = createBreadcrumbFetch(fetcher, trail);

    await recorded("https://tv.example.net/v1/channels/7/play-url?token=abc", { method: "post" });
    await recorded(new Request("https://tv.example.net/v1/guide?from=1", { method: "PUT" }));
    const failing = createBreadcrumbFetch(async () => Promise.reject(new TypeError("offline")), trail);
    await expect(failing("/v1/devices?x=1")).rejects.toThrow("offline");

    expect(fetcher).toHaveBeenCalledWith("https://tv.example.net/v1/channels/7/play-url?token=abc", {
      method: "post",
    });
    expect(trail.snapshot().map(({ category, message }) => `${category} ${message}`)).toEqual([
      "http POST /v1/channels/7/play-url 204",
      "http PUT /v1/guide 204",
      "http GET /v1/devices failed",
    ]);
  });
});

describe("crash reporter queue", () => {
  it("writes a fatal JS error to storage before returning, scrubbed and with breadcrumbs", () => {
    const { files, storage } = memoryStorage();
    const reporter = reporterAt(storage);
    reporter.breadcrumbs.add("navigation", "guide");
    const error = new TypeError("Cannot play https://tv.example.net/v1/hls/7.m3u8?token=abc");
    error.stack = "TypeError: Cannot play\n    at tune (https://tv.example.net/main.jsbundle?x=1:1:42)";

    reporter.captureError(error, true);

    expect(files.size).toBe(1);
    const [stored] = files.values();
    const report = JSON.parse(stored ?? "") as ClientCrashReportInput;
    expect(report).toEqual({
      ...device,
      breadcrumbs: [{ at: "2026-10-13T12:00:00.000Z", category: "navigation", message: "guide" }],
      kind: "js-fatal",
      message: "TypeError: Cannot play /v1/hls/7.m3u8",
      occurredAt: "2026-10-13T12:00:00.001Z",
      stack: "TypeError: Cannot play\n    at tune (/main.jsbundle:1:42)",
    });
  });

  it("bounds every field to the contract and never throws on odd thrown values", () => {
    const { storage } = memoryStorage();
    const reporter = new CrashReporter({
      device: { ...device, deviceModel: "m".repeat(300) },
      storage,
    });

    reporter.captureError(new Error("x".repeat(5000)), false);
    reporter.captureError("   ", false);
    const circular: { self?: unknown } = {};
    circular.self = circular;
    reporter.captureError(circular, false);

    const reports = reporter.pending();
    expect(reports.map(({ kind }) => kind)).toEqual(["js-error", "js-error", "js-error"]);
    expect(utf8Bytes(reports[0]?.message ?? "")).toBe(2048);
    expect(reports[0]?.deviceModel).toHaveLength(128);
    expect(reports[1]?.message).toBe("Unknown error");
    expect(reports[2]?.message).toBe("Unreadable thrown value");
  });

  it("keeps only the newest 20 reports", () => {
    const { files, storage } = memoryStorage();
    const reporter = reporterAt(storage);

    for (let index = 0; index < 25; index += 1) reporter.captureError(new Error(`crash ${index}`), true);

    expect(files.size).toBe(20);
    expect(reporter.pending().map(({ message }) => message)).toEqual(
      Array.from({ length: 20 }, (_, index) => `Error: crash ${index + 5}`),
    );
  });

  it("deletes a report only after a 2xx and stops at the first failure", async () => {
    const { storage } = memoryStorage();
    const reporter = reporterAt(storage);
    for (const message of ["one", "two", "three"]) reporter.captureError(new Error(message), true);
    const statuses = [202, 503];
    const send = vi.fn<CrashReportSend>(async () => statuses.shift() ?? 202);

    expect(await reporter.flush(send)).toEqual({ dropped: 0, kept: 2, sent: 1 });
    expect(send).toHaveBeenCalledTimes(2);
    expect(reporter.pending().map(({ message }) => message)).toEqual(["Error: two", "Error: three"]);

    const offline = vi.fn<CrashReportSend>(async () => Promise.reject(new TypeError("offline")));
    expect(await reporter.flush(offline)).toEqual({ dropped: 0, kept: 2, sent: 0 });
    for (const status of [401, 429]) {
      expect(await reporter.flush(async () => status)).toEqual({ dropped: 0, kept: 2, sent: 0 });
    }

    expect(await reporter.flush(send)).toEqual({ dropped: 0, kept: 0, sent: 2 });
    expect(reporter.pending()).toEqual([]);
  });

  it("drops a report the server refuses for good instead of resending it every launch", async () => {
    const { storage } = memoryStorage();
    const reporter = reporterAt(storage);
    reporter.captureError(new Error("too big"), true);
    reporter.captureError(new Error("fine"), true);
    const statuses = [413, 202];

    expect(await reporter.flush(async () => statuses.shift() ?? 500)).toEqual({
      dropped: 1,
      kept: 0,
      sent: 1,
    });
  });

  it("runs one flush at a time", async () => {
    const { storage } = memoryStorage();
    const reporter = reporterAt(storage);
    reporter.captureError(new Error("once"), true);
    const send = vi.fn<CrashReportSend>(async () => 202);

    const [first, second] = await Promise.all([reporter.flush(send), reporter.flush(send)]);

    expect(first).toBe(second);
    expect(send).toHaveBeenCalledTimes(1);
  });

  it("removes a torn or foreign entry instead of sending it", () => {
    const { files, storage } = memoryStorage();
    files.set("001760356800000-0abc.json", '{"kind":"js-fat');
    files.set("001760356800001-1abc.json", '{"hello":"world"}');
    files.set("unrelated.txt", "kept");
    const reporter = reporterAt(storage);

    expect(reporter.pending()).toEqual([]);
    expect([...files.keys()]).toEqual(["unrelated.txt"]);
  });
});

describe("global error handler", () => {
  type Handler = ReturnType<GlobalErrorUtils["getGlobalHandler"]>;
  const errorUtilsWith = (previous: Handler) => {
    let current = previous;
    return {
      getGlobalHandler: () => current,
      setGlobalHandler: (handler: Handler) => {
        current = handler;
      },
    } satisfies GlobalErrorUtils;
  };

  it("persists the crash before React Native's handler ends the app, then chains to it", () => {
    const { files, storage } = memoryStorage();
    const reporter = reporterAt(storage);
    const filesWhenPreviousRan: number[] = [];
    const previous = vi.fn(() => filesWhenPreviousRan.push(files.size));
    const errorUtils = errorUtilsWith(previous);

    const uninstall = installCrashHandler(errorUtils, reporter);
    const error = new Error("boom");
    errorUtils.getGlobalHandler()(error, true);

    expect(previous).toHaveBeenCalledWith(error, true);
    expect(filesWhenPreviousRan).toEqual([1]);
    expect(reporter.pending()[0]?.kind).toBe("js-fatal");

    uninstall();
    expect(errorUtils.getGlobalHandler()).toBe(previous);
  });

  it("still chains when the report cannot be written", () => {
    const { storage } = memoryStorage();
    const reporter = reporterAt({
      ...storage,
      write: () => {
        throw new Error("disk full");
      },
    });
    const previous = vi.fn();
    const errorUtils = errorUtilsWith(previous);
    installCrashHandler(errorUtils, reporter);

    errorUtils.getGlobalHandler()(new Error("boom"), false);

    expect(previous).toHaveBeenCalledWith(expect.any(Error), false);
  });
});

describe("crash report sender", () => {
  it("posts the report as JSON to the crash endpoint and answers the status", async () => {
    const fetcher = vi.fn<typeof fetch>(async () => new Response('{"id":"c1"}', { status: 202 }));
    const report = { ...device, breadcrumbs: [], kind: "js-fatal", message: "x", occurredAt: "now" } as const;

    expect(await createCrashReportSender(fetcher)(report)).toBe(202);
    expect(fetcher).toHaveBeenCalledWith("/v1/diagnostics/client-crashes", {
      body: JSON.stringify(report),
      headers: { "Content-Type": "application/json" },
      method: "POST",
    });
  });
});

describe("MetricKit forwarding", () => {
  type Frame = { [key: string]: unknown; subFrames?: Frame[] };
  const frames = (depth: number): Frame => ({
    address: 4_300_000_000,
    binaryName: "LoomarrMobile",
    binaryUUID: "0F4E3B0A-0000-4000-8000-000000000000",
    offsetIntoBinaryTextSegment: 1024,
    sampleCount: 1,
    ...(depth > 0 ? { subFrames: [frames(depth - 1)] } : {}),
  });
  const crash = (extra: Record<string, unknown> = {}) => ({
    callStackTree: {
      callStackPerThread: true,
      callStacks: [
        { callStackRootFrames: [frames(3)], threadAttributed: true },
        { callStackRootFrames: [frames(3)], threadAttributed: false },
      ],
    },
    diagnosticMetaData: {
      appBuildVersion: "6",
      appVersion: "0.2.0",
      deviceType: "iPhone16,2",
      exceptionCode: 0,
      exceptionType: 1,
      osVersion: "iPhone OS 27.0 (24A5)",
      signal: 11,
      terminationReason: "Namespace SIGNAL, Code 11 Segmentation fault: 11",
    },
    version: "1.0.0",
    ...extra,
  });
  const envelope = (payload: unknown) => ({
    endedAt: "2026-10-13T11:59:00Z",
    id: "A1B2",
    payload: JSON.stringify(payload),
  });

  it("queues one native report per crash with the crashed build's identity", () => {
    const { storage } = memoryStorage();
    const reporter = reporterAt(storage);
    const payload = {
      crashDiagnostics: [
        crash(),
        crash({
          diagnosticMetaData: {
            exceptionReason: {
              composedMessage: "Unhandled JS Exception: fetch https://tv.example.net/v1/x?token=abc failed",
            },
            exceptionType: 10,
            signal: 6,
          },
        }),
      ],
      hangDiagnostics: [{ hangDuration: "3 sec" }],
      timeStampBegin: "2026-10-12 12:00:00",
      timeStampEnd: "2026-10-13 11:59:00",
    };

    expect(reporter.captureNative(envelope(payload))).toBe(2);

    const [segfault, abort] = reporter.pending();
    expect(segfault).toMatchObject({
      appBuild: "6",
      appVersion: "0.2.0",
      breadcrumbs: [],
      deviceModel: "iPhone16,2",
      kind: "native",
      message: "EXC_BAD_ACCESS (SIGSEGV): Namespace SIGNAL, Code 11 Segmentation fault: 11",
      occurredAt: "2026-10-13T11:59:00Z",
      osVersion: "iPhone OS 27.0 (24A5)",
      platform: "ios",
    });
    expect(segfault?.nativePayload).toEqual({
      crashDiagnostics: [crash()],
      timeStampBegin: "2026-10-12 12:00:00",
      timeStampEnd: "2026-10-13 11:59:00",
    });
    // A crash MetricKit could not attribute to a build falls back to this launch's identity.
    expect(abort).toMatchObject({
      appBuild: "7",
      deviceModel: "iPhone17,1",
      message: "EXC_CRASH (SIGABRT): Unhandled JS Exception: fetch /v1/x failed",
    });
    expect(JSON.stringify(abort?.nativePayload)).not.toContain("token");
  });

  it("keeps a deep payload inside 64 KiB, dropping other threads before the crashed one", () => {
    const { storage } = memoryStorage();
    const reporter = reporterAt(storage);
    const deep = crash();
    deep.callStackTree.callStacks[1] = { callStackRootFrames: [frames(400)], threadAttributed: false };
    const huge = crash();
    huge.callStackTree.callStacks[0] = { callStackRootFrames: [frames(400)], threadAttributed: true };

    reporter.captureNative(envelope({ crashDiagnostics: [deep, huge] }));

    const [first, second] = reporter.pending();
    type Diagnostic = { callStackTree?: { callStacks: unknown[] } };
    const stacks = (report: ClientCrashReportInput | undefined) =>
      (report?.nativePayload as { crashDiagnostics?: Diagnostic[] } | undefined)?.crashDiagnostics?.[0]
        ?.callStackTree?.callStacks;
    expect(stacks(first)).toEqual([deep.callStackTree.callStacks[0]]);
    expect(stacks(second)).toBeUndefined();
    for (const report of [first, second]) {
      expect(utf8Bytes(JSON.stringify(report?.nativePayload))).toBeLessThanOrEqual(64 << 10);
    }
  });

  it("ignores a payload that is not MetricKit JSON or holds no crash", () => {
    const { files, storage } = memoryStorage();
    const reporter = reporterAt(storage);

    expect(reporter.captureNative({ endedAt: "2026-10-13T00:00:00Z", id: "x", payload: "{not json" })).toBe(
      0,
    );
    expect(reporter.captureNative(envelope({ hangDiagnostics: [{}] }))).toBe(0);
    expect(files.size).toBe(0);
  });
});
