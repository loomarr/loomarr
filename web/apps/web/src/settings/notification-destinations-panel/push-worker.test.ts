import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it, vi } from "vitest";

type WorkerListener = (event: Record<string, unknown>) => void;

const loadWorker = () => {
  const listeners = new Map<string, WorkerListener>();
  const showNotification = vi.fn(async () => undefined);
  const openWindow = vi.fn(async () => undefined);
  const worker = {
    addEventListener: (name: string, listener: WorkerListener) => listeners.set(name, listener),
    registration: { showNotification },
    clients: { matchAll: vi.fn(async () => []), openWindow },
  };
  const source = readFileSync(resolve(process.cwd(), "public/push-worker.js"), "utf8");
  Function("self", source)(worker);
  return { listeners, showNotification, openWindow };
};

describe("Browser Push service worker", () => {
  it("always displays the server-bounded preview", async () => {
    const { listeners, showNotification } = loadWorker();
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
    const { listeners, openWindow } = loadWorker();
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
