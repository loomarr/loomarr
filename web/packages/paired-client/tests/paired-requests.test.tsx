// @vitest-environment jsdom

import type { PairingCredential, PairingSession } from "@loomarr/core/pairing";
import { act, cleanup, renderUi } from "@loomarr/test-support";
import { createElement } from "react";
import { AppState } from "react-native";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("expo-crypto", () => ({ randomUUID: vi.fn() }));
vi.mock("expo-video", () => ({ createVideoPlayer: vi.fn(), VideoView: vi.fn() }));

const { createPairedEvents, usePairedRequests } = await import("@loomarr/paired-client");

// The app's foreground and background, delivered to whoever listens, as the system would.
const appListeners = new Set<(state: string) => void>();
beforeEach(() => {
  vi.spyOn(AppState, "addEventListener").mockImplementation(((
    _type: string,
    listener: (state: string) => void,
  ) => {
    appListeners.add(listener);
    return { remove: () => appListeners.delete(listener) };
  }) as never);
});

// A root a test renders into more than once, unmounted by `cleanup`.
const testRoot = () => {
  const { rerender, unmount } = renderUi(null);
  return { render: rerender, unmount };
};

const credential: PairingCredential = {
  deviceName: "iPhone Loomarr",
  expiresAt: "2027-01-01T00:00:00Z",
  serverUrl: "https://loomarr.test",
  token: "device-token",
} as PairingCredential;
const session = { revoked: vi.fn() } as unknown as PairingSession;

const json = (body: unknown) => new Response(JSON.stringify(body), { status: 200 });

// One request in each state: only the failed one needs the person, whoever they are.
const journeys = [
  { jobId: "ok", milestone: "live" },
  { jobId: "bad", milestone: "failed" },
].map((journey) => ({
  actions: [],
  attempts: [],
  createdAt: "2026-10-01T18:00:00Z",
  intent: { description: journey.jobId },
  updatedAt: "2026-10-01T18:00:00Z",
  version: 1,
  ...journey,
}));

/** The server as one role sees it: every URL the Requests controller reads, recorded with its headers. */
const serve = (role: "admin" | "member") => {
  const calls: { authorization: string | null; path: string }[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: string | Request, init?: RequestInit) => {
      const url = new URL(typeof input === "string" ? input : input.url);
      calls.push({
        authorization: new Headers(init?.headers).get("Authorization"),
        path: url.pathname,
      });
      if (url.pathname === "/v1/auth/me") return json({ role });
      if (url.pathname === "/v1/proposal-jobs") return json({ journeys });
      if (url.pathname === "/v1/proposals") return json({ proposals: [{ id: "p1" }, { id: "p2" }] });
      if (url.pathname === "/v1/filler/pulls") return json({ pulls: [{ id: "f1" }] });
      return json({ titles: [] });
    }),
  );
  return calls;
};

const mount = async () => {
  const events = createPairedEvents();
  const seen: ReturnType<typeof usePairedRequests>[] = [];
  const Probe = () => {
    seen.push(usePairedRequests({ credential, events, session }));
    return null;
  };
  const root = testRoot();
  await act(async () => root.render(createElement(Probe)));
  await act(async () => undefined);
  return {
    events,
    latest: () => seen[seen.length - 1] as ReturnType<typeof usePairedRequests>,
    rerender: () => act(async () => root.render(createElement(Probe))),
    unmount: () => act(async () => root.unmount()),
  };
};

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  appListeners.clear();
});

const journeyReads = (calls: { path: string }[]) =>
  calls.filter(({ path }) => path === "/v1/proposal-jobs").length;

describe("paired requests", () => {
  it("counts an admin's pending decisions and failed requests before Requests is opened", async () => {
    const calls = serve("admin");
    const requests = await mount();

    // 2 proposals + 1 filler pull + 1 failed request.
    expect(requests.latest().needsYouCount).toBe(4);
    expect(requests.latest().controller.getSnapshot().role).toBe("admin");
    expect(calls.every(({ authorization }) => authorization === "Bearer device-token")).toBe(true);
    await requests.unmount();
  });

  it("gives a member only their failed requests and never reads the admin lists", async () => {
    const calls = serve("member");
    const requests = await mount();

    expect(requests.latest().needsYouCount).toBe(1);
    expect(requests.latest().controller.getSnapshot()).toMatchObject({ approvals: [], role: "member" });
    const paths = calls.map(({ path }) => path);
    expect(paths).toContain("/v1/auth/me");
    expect(paths).not.toContain("/v1/proposals");
    expect(paths).not.toContain("/v1/filler/pulls");
    await requests.unmount();
  });

  // A request moves on the server: the phone reads it again on the frames that announce it, not
  // on a timer (#3352).
  it.each(["proposal", "title", "channel"] as const)(
    "reads the requests again on a %s frame",
    async (type) => {
      const calls = serve("member");
      const requests = await mount();
      const before = journeyReads(calls);

      await act(async () => requests.events.onFrame(type, {} as never));
      expect(journeyReads(calls)).toBe(before + 1);
      await requests.unmount();
      await act(async () => requests.events.onFrame(type, {} as never));
      expect(journeyReads(calls)).toBe(before + 1);
    },
  );

  // Every phone hears every member's run; only the person's own runs are worth a read each step.
  it("reads again on the progress of the person's own run, not another member's", async () => {
    const calls = serve("member");
    const requests = await mount();
    const before = journeyReads(calls);

    await act(async () =>
      requests.events.onFrame("suggestion", { jobId: "someone-else", phase: "scoring", round: 1 }),
    );
    expect(journeyReads(calls)).toBe(before);
    await act(async () =>
      requests.events.onFrame("suggestion", { jobId: "bad", phase: "scoring", round: 1 }),
    );
    expect(journeyReads(calls)).toBe(before + 1);
    await requests.unmount();
  });

  it("reads again as the stream comes back and as the app returns to the foreground", async () => {
    const calls = serve("member");
    const requests = await mount();
    const before = journeyReads(calls);

    await act(async () => requests.events.resync());
    expect(journeyReads(calls)).toBe(before + 1);
    await act(async () => {
      for (const listener of [...appListeners]) listener("background");
    });
    expect(journeyReads(calls)).toBe(before + 1);
    await act(async () => {
      for (const listener of [...appListeners]) listener("active");
    });
    expect(journeyReads(calls)).toBe(before + 2);
    await requests.unmount();
  });

  it("stays subscribed once across renders", async () => {
    serve("member");
    const requests = await mount();
    const subscribe = vi.spyOn(requests.events, "subscribe");

    await requests.rerender();
    await requests.rerender();
    expect(subscribe).not.toHaveBeenCalled();
    await requests.unmount();
  });
});
