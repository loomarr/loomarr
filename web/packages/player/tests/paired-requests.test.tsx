// @vitest-environment jsdom

import type { PairingCredential, PairingSession } from "@loomarr/core/pairing";
import { act, createElement } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, describe, expect, it, vi } from "vitest";

vi.mock("expo-crypto", () => ({ randomUUID: vi.fn() }));
vi.mock("expo-video", () => ({ createVideoPlayer: vi.fn(), VideoView: vi.fn() }));

const { usePairedRequests } = await import("@loomarr/player/native");

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

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
  const seen: ReturnType<typeof usePairedRequests>[] = [];
  const Probe = () => {
    seen.push(usePairedRequests({ credential, session }));
    return null;
  };
  const container = (
    globalThis as unknown as { document: { createElement: (tag: string) => never } }
  ).document.createElement("div");
  const root = createRoot(container);
  await act(async () => root.render(createElement(Probe)));
  await act(async () => undefined);
  return {
    latest: () => seen[seen.length - 1] as ReturnType<typeof usePairedRequests>,
    unmount: () => act(async () => root.unmount()),
  };
};

afterEach(() => vi.unstubAllGlobals());

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
});
