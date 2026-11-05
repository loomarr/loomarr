import { afterEach, describe, expect, it, vi } from "vitest";

type WorkerListener = (event: Record<string, unknown>) => void;

// The worker is a classic script served from public/, registered by URL. The test runs the real
// file against a stand-in `self`: a fresh module instance per test registers fresh listeners.
const WORKER: string = "../../../public/push-worker.js";

const loadWorker = async () => {
  const listeners = new Map<string, WorkerListener>();
  const showNotification = vi.fn(async () => undefined);
  const openWindow = vi.fn(async () => undefined);
  vi.stubGlobal("self", {
    addEventListener: (name: string, listener: WorkerListener) => listeners.set(name, listener),
    registration: { showNotification },
    clients: { matchAll: vi.fn(async () => []), openWindow },
  });
  vi.resetModules();
  // A specifier held in a variable: the worker is plain JavaScript with no type declarations.
  await import(/* @vite-ignore */ WORKER);
  return { listeners, showNotification, openWindow };
};

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("Browser Push service worker", () => {
  it("always displays the server-bounded preview", async () => {
    const { listeners, showNotification } = await loadWorker();
    let work: Promise<unknown> | undefined;
    listeners.get("push")?.({
      data: {
        json: () => ({
          title: "Loomarr",
          body: "You have a new Loomarr notification.",
          url: "/queue/flight",
          tag: "loomarr-event-1",
        }),
      },
      waitUntil: (promise: Promise<unknown>) => {
        work = promise;
      },
    });
    await work;
    expect(showNotification).toHaveBeenCalledWith(
      "Loomarr",
      expect.objectContaining({
        body: "You have a new Loomarr notification.",
        data: { url: "/queue/flight" },
      }),
    );
  });

  it("opens only a same-origin path when the notification is clicked", async () => {
    const { listeners, openWindow } = await loadWorker();
    let work: Promise<unknown> | undefined;
    listeners.get("notificationclick")?.({
      notification: { close: vi.fn(), data: { url: "//attacker.example/path" } },
      waitUntil: (promise: Promise<unknown>) => {
        work = promise;
      },
    });
    await work;
    expect(openWindow).toHaveBeenCalledWith("/");
  });
});
