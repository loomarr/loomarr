// @vitest-environment jsdom

import { createPairedServer } from "@loomarr/fixtures";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

// What the device holds, and what the TV's native modules would answer: the app's own code runs as is,
// against a paired server that answers from the test card (`createPairedServer`).
const device = vi.hoisted(() => {
  Object.assign(globalThis, {
    __DEV__: false,
    ErrorUtils: { getGlobalHandler: () => undefined, setGlobalHandler: () => undefined },
  });
  return {
    idleDiscovery: { servers: [], status: "idle" },
    store: new Map<string, string>(),
  };
});

vi.mock("expo-keep-awake", () => ({ useKeepAwake: () => undefined }));
vi.mock("expo-splash-screen", () => ({ hide: () => undefined, preventAutoHideAsync: async () => true }));
vi.mock("expo-status-bar", () => ({ StatusBar: () => null }));
vi.mock("expo-crypto", () => ({ randomUUID: () => "11111111-2222-4333-8444-555555555555" }));
vi.mock("expo-constants", () => ({ default: { expoConfig: { version: "0.0.0-test" } } }));
vi.mock("expo-secure-store", () => ({
  deleteItemAsync: async (key: string) => void device.store.delete(key),
  getItemAsync: async (key: string) => device.store.get(key) ?? null,
  setItemAsync: async (key: string, value: string) => void device.store.set(key, value),
}));
vi.mock("expo-video", () => ({ createVideoPlayer: vi.fn(), VideoView: vi.fn() }));
vi.mock("react-native-safe-area-context", () => ({
  SafeAreaProvider: ({ children }: { children: unknown }) => children,
  useSafeAreaInsets: () => ({ bottom: 0, left: 0, right: 0, top: 0 }),
}));
vi.mock("@loomarr/lan-discovery-native", () => ({
  // A paired TV never browses the LAN; the shell only stops a search it never started.
  createNativeServerDiscovery: () => ({
    snapshot: () => device.idleDiscovery,
    start: () => undefined,
    stop: () => undefined,
    subscribe: () => () => undefined,
  }),
}));
vi.mock("@loomarr/crash-reports-native", () => ({
  createNativeCrashSource: () => undefined,
  forwardNativeCrashes: async () => 0,
}));
vi.mock("expo-file-system", () => {
  class Directory {
    exists = false;
    create() {}
    list() {
      return [];
    }
  }
  class File {
    exists = false;
    delete() {}
    textSync() {
      return "";
    }
    writeSync() {}
  }
  return { Directory, File, Paths: { document: "/documents" } };
});
// react-native-web, as an Android TV: it has no TV remote, so its keys are not sent here.
vi.mock("react-native", async (importOriginal) => {
  const original = await importOriginal<typeof import("react-native")>();
  return {
    ...original,
    Platform: {
      ...original.Platform,
      OS: "android",
      Version: 34,
      constants: { Model: "Test TV", Release: "14" },
      isTV: true,
    },
    TVEventControl: { disableTVMenuKey: () => undefined, enableTVMenuKey: () => undefined },
    useTVEventHandler: () => undefined,
  };
});
vi.mock("@loomarr/player/native", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@loomarr/player/native")>()),
  NativePlayerView: () => null,
  PairedNativeImage: () => null,
  createExpoVideoTransport: () => ({
    dispose: () => undefined,
    goLive: () => undefined,
    pause: () => undefined,
    play: () => undefined,
    replace: async () => undefined,
    resume: () => undefined,
    subscribe: () => () => undefined,
    suspend: () => undefined,
  }),
}));

type Node = {
  click(): void;
  focus(): void;
  getAttribute(name: string): string | null;
  textContent: string | null;
};
type Document = {
  activeElement: Node | null;
  body: {
    append(node: unknown): void;
    querySelector(selector: string): Node | null;
    querySelectorAll(selector: string): ArrayLike<Node>;
  };
  createElement(tag: string): Parameters<typeof createRoot>[0];
};
const page = (globalThis as unknown as { document: Document }).document;

let server: ReturnType<typeof createPairedServer>;
let root: Root;

const label = (prefix: string) => page.body.querySelector(`[aria-label^="${prefix}"]`);
// The Guide notice's button, which reads "Trying again…" while its retry is in flight.
const tryAgain = () =>
  Array.from(page.body.querySelectorAll('[role="button"]')).find((node) =>
    /^Try(ing)? again/.test(node.textContent?.trim() ?? ""),
  ) ?? null;
const focusedLabel = () => page.activeElement?.getAttribute("aria-label");
const eventually = (check: () => void) => vi.waitFor(check, { timeout: 8_000 });

beforeEach(() => {
  server = createPairedServer();
  vi.stubGlobal("innerWidth", 1280);
  vi.stubGlobal("XMLHttpRequest", server.EventRequest);
  vi.stubGlobal("fetch", vi.fn(server.fetch));
  // The TV was paired on an earlier launch.
  device.store.set(
    "loomarr.paired-device.v1",
    JSON.stringify({ deviceName: "Loomarr TV", serverUrl: server.serverUrl, token: "device-token" }),
  );
});

afterEach(async () => {
  await act(async () => root.unmount());
  vi.unstubAllGlobals();
  device.store.clear();
});

const { default: App } = await import("../src/app");

const launch = async () => {
  const host = page.createElement("div");
  page.body.append(host);
  root = createRoot(host);
  await act(async () => root.render(<App />));
};

describe("the TV app", () => {
  it("opens a paired TV on its first channel, with the server's channels loaded", async () => {
    await launch();

    await eventually(() => expect(label("Open programme guide")).not.toBeNull());
    expect(server.requests).toContain("GET /v1/channels");
    expect(server.requests).toContain("POST /v1/channels/ch-cartoons/play-url");
  });

  it("keeps the Guide's grid on screen, and the remote on the same channel, when the server announces a channel change", async () => {
    await launch();
    await eventually(() => expect(label("Open programme guide")).not.toBeNull());
    await act(async () => label("Open programme guide")?.click());
    await eventually(() => expect(label("Sci-Fi Classics, ")).not.toBeNull());

    // The viewer has moved down to Sci-Fi Classics, which is not the channel on screen.
    await act(async () => label("Sci-Fi Classics, ")?.focus());
    expect(focusedLabel()).toMatch(/^Sci-Fi Classics, /);
    const loadsBefore = server.guideLoads();

    // The frame makes the Guide load again; while the server is still answering, the grid stays.
    const answerGuide = server.holdGuide();
    await act(async () => server.sendFrame("channel", { id: "ch-scifi", kind: "updated" }));
    await eventually(() => expect(server.guideLoads()).toBeGreaterThan(loadsBefore));
    expect(label("Programme guide")).not.toBeNull();
    expect(label("Sci-Fi Classics, ")).not.toBeNull();
    expect(focusedLabel()).toMatch(/^Sci-Fi Classics, /);

    await act(async () => answerGuide());
    expect(label("Programme guide")).not.toBeNull();
    expect(label("Cartoon Classics, ")).not.toBeNull();
    expect(focusedLabel()).toMatch(/^Sci-Fi Classics, /);
  });

  // The remote's place is the notice's button: if a retry unmounted the notice, the remote would
  // have nothing focused and no way back to the Guide without a restart of the gesture.
  it("keeps the remote on the Guide notice's Try again while it retries, and when the retry fails", async () => {
    await launch();
    await eventually(() => expect(label("Open programme guide")).not.toBeNull());
    await act(async () => label("Open programme guide")?.click());
    await eventually(() => expect(label("Sci-Fi Classics, ")).not.toBeNull());

    // A channel frame makes the Guide load again, and the server is down: the grid stays, said stale.
    const restore = server.failGuide();
    await act(async () => server.sendFrame("channel", { id: "ch-scifi", kind: "updated" }));
    await eventually(() => expect(tryAgain()).not.toBeNull());
    const button = tryAgain();
    await act(async () => button?.focus());
    expect(page.activeElement).toBe(button);

    // The retry is on its way: the same button says so, and the remote is still on it.
    const answer = server.holdGuide();
    await act(async () => button?.click());
    expect(tryAgain()).toBe(button);
    expect(button?.textContent).toContain("Trying again");
    expect(page.activeElement).toBe(button);
    expect(label("Sci-Fi Classics, ")).not.toBeNull();

    // It fails again: the notice is back as it was, and the remote never left it.
    await act(async () => answer());
    await eventually(() => expect(button?.textContent).not.toContain("Trying again"));
    expect(tryAgain()).toBe(button);
    expect(page.activeElement).toBe(button);

    // The server is back: the notice goes, the grid stays.
    restore();
    await act(async () => button?.click());
    await eventually(() => expect(tryAgain()).toBeNull());
    expect(label("Sci-Fi Classics, ")).not.toBeNull();
  });
});
