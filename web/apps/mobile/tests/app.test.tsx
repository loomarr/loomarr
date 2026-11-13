// @vitest-environment jsdom

import { createPairedServer } from "@loomarr/fixtures";
import { NativeAppRoot } from "@loomarr/native-app";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

// What the phone holds, and what its native modules would answer: the app's own code runs as is,
// against a paired server that answers from the test card (`createPairedServer`).
const device = vi.hoisted(() => {
  Object.assign(globalThis, {
    __DEV__: false,
    ErrorUtils: { getGlobalHandler: () => undefined, setGlobalHandler: () => undefined },
  });
  return { store: new Map<string, string>() };
});

vi.mock("expo-status-bar", () => ({ StatusBar: () => null }));
vi.mock("expo-crypto", () => ({ randomUUID: () => "11111111-2222-4333-8444-555555555555" }));
vi.mock("expo-constants", () => ({
  default: { expoConfig: { ios: { buildNumber: "7" }, version: "0.0.0-test" } },
}));
vi.mock("expo-haptics", () => ({
  ImpactFeedbackStyle: { Light: "light" },
  NotificationFeedbackType: { Success: "success" },
  impactAsync: async () => undefined,
  notificationAsync: async () => undefined,
  selectionAsync: async () => undefined,
}));
vi.mock("expo-screen-orientation", () => ({
  OrientationLock: { DEFAULT: 0, PORTRAIT_UP: 1 },
  lockAsync: async () => undefined,
  unlockAsync: async () => undefined,
}));
vi.mock("expo-web-browser", () => ({
  WebBrowserPresentationStyle: { PAGE_SHEET: "pageSheet" },
  dismissBrowser: () => undefined,
  openBrowserAsync: async () => ({ type: "opened" }),
}));
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
vi.mock("@loomarr/lan-discovery-native", () => ({ createNativeServerDiscovery: () => undefined }));
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
// react-native-web, as an Android phone.
vi.mock("react-native", async (importOriginal) => {
  const original = await importOriginal<typeof import("react-native")>();
  return {
    ...original,
    Platform: {
      ...original.Platform,
      OS: "android",
      Version: 34,
      constants: { Model: "Test Phone", Release: "14" },
      isTV: false,
    },
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

type Node = { click(): void; focus(): void; textContent: string | null };
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

// On the phone a programme reads "<channel number> <channel>, <programme>, <time>".
const label = (part: string) => page.body.querySelector(`[aria-label*="${part}"]`);
const eventually = (check: () => void) => vi.waitFor(check, { timeout: 8_000 });
// The Guide notice's button, which reads "Trying again…" while its retry is in flight.
const tryAgain = () =>
  Array.from(page.body.querySelectorAll('[role="button"]')).find((node) =>
    /^Try(ing)? again/.test(node.textContent?.trim() ?? ""),
  ) ?? null;

beforeEach(() => {
  server = createPairedServer();
  vi.stubGlobal("XMLHttpRequest", server.EventRequest);
  vi.stubGlobal("fetch", vi.fn(server.fetch));
  // The phone was paired on an earlier launch.
  device.store.set(
    "loomarr.paired-device.v1",
    JSON.stringify({ deviceName: "Android Loomarr", serverUrl: server.serverUrl, token: "device-token" }),
  );
});

afterEach(async () => {
  await act(async () => root.unmount());
  Reflect.deleteProperty(globalThis.document, "visibilityState");
  vi.unstubAllGlobals();
  device.store.clear();
});

// The app going to the background and coming back, as react-native-web reports it: the page's
// visibility.
const setVisibility = async (state: "hidden" | "visible") => {
  const target = globalThis as unknown as {
    document: object & { dispatchEvent(event: unknown): boolean };
    Event: new (type: string) => unknown;
  };
  Object.defineProperty(target.document, "visibilityState", { configurable: true, value: state });
  await act(async () => void target.document.dispatchEvent(new target.Event("visibilitychange")));
};

const { default: Index } = await import("../app/index");
const { nativeApp } = await import("../src/native-app");

// The route's own layout (app/_layout.tsx) draws the root around the screen; the router is not under test.
const launch = async () => {
  const host = page.createElement("div");
  page.body.append(host);
  root = createRoot(host);
  await act(async () =>
    root.render(
      <NativeAppRoot app={nativeApp}>
        <Index />
      </NativeAppRoot>,
    ),
  );
};

describe("the phone app", () => {
  it("opens a paired phone on its Guide, loaded from the server, with nothing tuned", async () => {
    await launch();

    await eventually(() => expect(label("Sci-Fi Classics, ")).not.toBeNull());
    expect(label("Cartoon Classics, ")).not.toBeNull();
    expect(server.requests).toContain("GET /v1/channels");
    expect(server.requests.filter((request) => request.endsWith("/play-url"))).toEqual([]);
  });

  // A phone left in a pocket overnight shows yesterday's window until the Guide is read again.
  it("reads the Guide again when the app returns to the foreground", async () => {
    await launch();
    await eventually(() => expect(label("Sci-Fi Classics, ")).not.toBeNull());
    await setVisibility("hidden");
    const before = server.guideLoads();

    await setVisibility("visible");

    await eventually(() => expect(server.guideLoads()).toBeGreaterThan(before));
  });

  it("keeps focus on the Guide notice's Try again while it retries, and when the retry fails", async () => {
    await launch();
    await eventually(() => expect(label("Sci-Fi Classics, ")).not.toBeNull());

    const restore = server.failGuide();
    await act(async () => server.sendFrame("channel", { id: "ch-scifi", kind: "updated" }));
    await eventually(() => expect(tryAgain()).not.toBeNull());
    const button = tryAgain();
    await act(async () => button?.focus());
    expect(page.activeElement).toBe(button);

    const answer = server.holdGuide();
    await act(async () => button?.click());
    expect(tryAgain()).toBe(button);
    expect(button?.textContent).toContain("Trying again");
    expect(page.activeElement).toBe(button);
    expect(label("Sci-Fi Classics, ")).not.toBeNull();

    await act(async () => answer());
    await eventually(() => expect(button?.textContent).not.toContain("Trying again"));
    expect(tryAgain()).toBe(button);
    expect(page.activeElement).toBe(button);

    restore();
    await act(async () => button?.click());
    await eventually(() => expect(tryAgain()).toBeNull());
    expect(label("Sci-Fi Classics, ")).not.toBeNull();
  });

  // The phone has no remote to send, so a retry that failed and a frame that later recovers the
  // Guide must take the notice away and leave the viewer wherever they were.
  it("takes the notice away without moving focus when a frame recovers the Guide after a retry that failed", async () => {
    await launch();
    await eventually(() => expect(label("Sci-Fi Classics, ")).not.toBeNull());

    const restore = server.failGuide();
    await act(async () => server.sendFrame("channel", { id: "ch-scifi", kind: "updated" }));
    await eventually(() => expect(tryAgain()).not.toBeNull());
    const button = tryAgain();
    await act(async () => button?.focus());
    const answer = server.holdGuide();
    await act(async () => button?.click());
    await act(async () => answer());
    await eventually(() => expect(button?.textContent).not.toContain("Trying again"));

    await act(async () => (page.activeElement as unknown as { blur(): void }).blur());
    restore();
    await act(async () => server.sendFrame("channel", { id: "ch-scifi", kind: "updated" }));
    await eventually(() => expect(tryAgain()).toBeNull());
    expect(label("Sci-Fi Classics, ")).not.toBeNull();
    expect(page.activeElement?.getAttribute("aria-label") ?? null).toBeNull();
  });

  it("tunes the channel the viewer picks in the Guide", async () => {
    await launch();
    await eventually(() => expect(label("Watch ")).not.toBeNull());

    await act(async () => label("Watch ")?.click());

    await eventually(() => expect(server.requests).toContain("POST /v1/channels/ch-action/play-url"));
  });
});
