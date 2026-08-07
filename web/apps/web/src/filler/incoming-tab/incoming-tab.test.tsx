import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { IncomingTab } from "./incoming-tab";

// IncomingTab owns the review queue and its four filing mutations. It was extracted from
// `filler-page.tsx`, where all of it mounted on every tab — so a member reading the Catalog
// still paid for a queue that is admin-only server-side.
//
// ⚠ These tests assert the REQUEST BODIES, not just that a click did something. The BE's
// `UpdateClipTags` writes era and audience unconditionally, so a confirm that sends a bare
// `{era}` silently wipes audience. That is a data-loss bug no rendering assertion can see, and
// this call site reproduced it once already. `category` is NOT part of the body (§10 V45a): it
// is a derived shadow of the taxonomy tags, and this confirm never touches tags at all.

const jsonResponse = (status: number, body: unknown) =>
  new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });

const ASK = {
  path: "a3/f9/abc.mp4",
  hash: "hash-abc",
  name: "Toy ad",
  suggestedEra: 1993,
  audience: "kids",
  category: "toys",
  confidence: 80,
  durationMs: 30_000,
};

const stubFetch = (incoming: Record<string, unknown> = {}) => {
  const calls: { method: string; url: string; body: unknown }[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string, init?: RequestInit) => {
      const method = init?.method ?? "GET";
      const u = String(url);
      calls.push({ method, url: u, body: init?.body ? JSON.parse(init.body as string) : undefined });
      if (u.includes("/v1/filler/incoming")) {
        return Promise.resolve(
          jsonResponse(200, { asks: [ASK], reels: [], recentlyFiled: [], total: 1, ...incoming }),
        );
      }
      return Promise.resolve(jsonResponse(200, {}));
    }),
  );
  return calls;
};

const makeWrapper = () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
};

const renderTab = (onEditTags = vi.fn()) => {
  render(<IncomingTab onEditTags={onEditTags} />, { wrapper: makeWrapper() });
  return { onEditTags };
};

afterEach(() => vi.unstubAllGlobals());

describe("IncomingTab", () => {
  it("fetches the incoming queue itself rather than being handed it", async () => {
    const calls = stubFetch();
    renderTab();
    // The whole point of the extraction: the tab is self-sufficient, so the shell no longer
    // unpacks a queue for a tab that may not be showing.
    await waitFor(() => expect(calls.some((c) => c.url.includes("/v1/filler/incoming"))).toBe(true));
    expect(await screen.findByText("Toy ad")).toBeInTheDocument();
  });

  // ⚠ THE regression this file exists for. A bare `{era}` blanks audience on the server, so the
  // confirm must carry the clip's current audience alongside the era. `category` must NOT be in
  // the body — it's a derived shadow (§10 V45a), and `PatchClipInputBody` has no such field any
  // more; sending it would be a stale field the server ignores at best.
  it("confirming an era sends audience too, never era alone, and never category", async () => {
    const calls = stubFetch();
    renderTab();
    await screen.findByText("Toy ad");

    // "Looks right" is the CONFIRM affordance for a guessed era (the panel's own copy).
    await userEvent.click(await screen.findByRole("button", { name: /looks right/i }));

    await waitFor(() => {
      const patch = calls.find((c) => c.method === "PATCH");
      // §10 V45a: the clip is identified by `hash` in the body (no {id} URL segment) — this
      // PATCH goes through the single-clip tag route, which is hash-keyed. `ask.path` stays
      // the local UI key for `onEditTags`/`busyClip` (see the test below and the mixed-model
      // note in incoming-tab.tsx itself).
      expect(patch?.body).toEqual({ hash: "hash-abc", era: 1993, audience: "kids" });
    });
  });

  it("hands tag editing up to the shell, which owns the one dialog", async () => {
    stubFetch();
    const { onEditTags } = renderTab();
    await screen.findByText("Toy ad");

    // For a clip with a GUESSED era the edit affordance reads "Not right" — the panel offers
    // "Add tags" only when there is no guess to reject.
    await userEvent.click(await screen.findByRole("button", { name: /not right/i }));

    expect(onEditTags).toHaveBeenCalledWith(ASK.path);
  });

  it("renders an empty queue without erroring", async () => {
    stubFetch({ asks: [], total: 0 });
    renderTab();
    await waitFor(() => expect(screen.queryByText("Toy ad")).not.toBeInTheDocument());
  });
});
