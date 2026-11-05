// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from "vitest";

const native = vi.hoisted(() => ({
  files: new Map<string, string>(),
  handler: undefined as ((error: Error, fatal?: boolean) => void) | undefined,
  isTV: false,
  os: "ios",
  previous: vi.fn(),
}));

vi.mock("react-native", () => ({
  AppState: { addEventListener: () => ({ remove: () => undefined }) },
  Platform: {
    constants: { Model: "Model", Release: "27.0" },
    get OS() {
      return native.os;
    },
    get isTV() {
      return native.isTV;
    },
    Version: "27.0",
  },
  Settings: { get: () => undefined },
}));
vi.mock("react-native-safe-area-context", () => ({
  SafeAreaProvider: ({ children }: { children: unknown }) => children,
  useSafeAreaInsets: () => ({ bottom: 0, left: 0, right: 0, top: 0 }),
}));
vi.mock("@loomarr/design-system", () => ({
  LoomarrProvider: ({ children }: { children: unknown }) => children,
}));
vi.mock("@loomarr/crash-reports-native", () => ({
  createNativeCrashSource: () => undefined,
  forwardNativeCrashes: async () => 0,
}));
vi.mock("expo-constants", () => ({
  default: { expoConfig: { android: { versionCode: 9 }, ios: { buildNumber: "7" }, version: "0.4.0" } },
}));
vi.mock("expo-file-system", () => {
  class Directory {
    exists = true;
    create() {}
    list() {
      return [...native.files.keys()].map((name) => ({ name }));
    }
  }
  class File {
    constructor(
      _directory: unknown,
      private readonly name: string,
    ) {}
    get exists() {
      return native.files.has(this.name);
    }
    delete() {
      native.files.delete(this.name);
    }
    textSync() {
      return native.files.get(this.name) ?? "";
    }
    writeSync(contents: string) {
      native.files.set(this.name, contents);
    }
  }
  return { Directory, File, Paths: { document: "/documents" } };
});

const {
  createNativeApp,
  nativeCrashPlatform,
  nativeDiagnosticsIdentity,
  NativeAppRoot,
  useCrashReportUpload,
} = await import("@loomarr/native-app");

const installErrorUtils = () => {
  native.handler = undefined;
  (globalThis as { ErrorUtils?: unknown }).ErrorUtils = {
    getGlobalHandler: () => native.handler ?? native.previous,
    setGlobalHandler: (handler: typeof native.handler) => {
      native.handler = handler;
    },
  };
};

afterEach(() => {
  native.files.clear();
  native.previous.mockClear();
  native.isTV = false;
  native.os = "ios";
});

describe("createNativeApp", () => {
  it("installs the crash handler, so a fatal error is stored and still reaches the previous handler", () => {
    installErrorUtils();
    createNativeApp();

    expect(native.handler).toBeDefined();
    const error = new Error("boom");
    native.handler?.(error, true);

    expect(native.previous).toHaveBeenCalledWith(error, true);
    const stored = [...native.files.values()].map((contents) => JSON.parse(contents));
    expect(stored).toHaveLength(1);
    expect(stored[0]).toMatchObject({ kind: "js-fatal", platform: "ios" });
  });

  it("reports a crash on tvOS as tvos and diagnoses it as apple_tv", () => {
    installErrorUtils();
    native.isTV = true;
    const app = createNativeApp({ clientVersion: "9.9.9" });

    native.handler?.(new Error("boom"), true);

    expect(app.diagnostics).toEqual({ clientVersion: "9.9.9", platform: "apple_tv", source: "apple_tv" });
    expect(JSON.parse([...native.files.values()][0] ?? "{}")).toMatchObject({ platform: "tvos" });
  });

  it("answers the app's configured version when none is given", () => {
    installErrorUtils();
    native.os = "android";
    expect(createNativeApp().diagnostics).toEqual({
      clientVersion: "0.4.0",
      platform: "android",
      source: "android",
    });
  });
});

describe("native platform derivation", () => {
  it("names tvOS apple_tv, never android_tv", () => {
    expect(nativeDiagnosticsIdentity({ isTV: true, os: "ios" }, "1.2.3")).toEqual({
      clientVersion: "1.2.3",
      platform: "apple_tv",
      source: "apple_tv",
    });
    expect(nativeCrashPlatform({ isTV: true, os: "ios" })).toBe("tvos");
  });

  it("names every other native runtime by its own platform", () => {
    expect(nativeDiagnosticsIdentity({ isTV: true, os: "android" }, "1")).toMatchObject({
      platform: "android_tv",
      source: "android_tv",
    });
    expect(nativeDiagnosticsIdentity({ isTV: false, os: "ios" }, "1")).toMatchObject({
      platform: "ios",
      source: "ios",
    });
    expect(nativeDiagnosticsIdentity({ isTV: false, os: "android" }, "1")).toMatchObject({
      platform: "android",
      source: "android",
    });
    expect(nativeCrashPlatform({ isTV: true, os: "android" })).toBe("android-tv");
    expect(nativeCrashPlatform({ isTV: false, os: "ios" })).toBe("ios");
    expect(nativeCrashPlatform({ isTV: false, os: "android" })).toBe("android");
  });
});

describe("useCrashReportUpload", () => {
  it("sends the crash the last launch stored to the paired server, once mounted under Root", async () => {
    installErrorUtils();
    const send = vi.fn(async () => new Response(null, { status: 202 }));
    globalThis.fetch = send as unknown as typeof fetch;
    const { act, createElement } = await import("react");
    const { createRoot } = await import("react-dom/client");
    (globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

    const app = createNativeApp();
    native.handler?.(new Error("boom"), true);
    expect(native.files.size).toBe(1);

    const Probe = () => {
      useCrashReportUpload(
        { deviceName: "Test", serverUrl: "https://loomarr.test", token: "t" } as never,
        { revoked: () => undefined } as never,
      );
      return null;
    };
    const root = createRoot(document.createElement("div"));
    await act(async () => root.render(createElement(NativeAppRoot, { app, children: createElement(Probe) })));
    await vi.waitFor(() => expect(send).toHaveBeenCalledOnce());

    const [url, init] = send.mock.calls[0] as unknown as [string, RequestInit];
    expect(url).toBe("https://loomarr.test/v1/diagnostics/client-crashes");
    expect(JSON.parse(init.body as string)).toMatchObject({ kind: "js-fatal", message: "Error: boom" });
    await vi.waitFor(() => expect(native.files.size).toBe(0));
    await act(async () => root.unmount());
  });
});
