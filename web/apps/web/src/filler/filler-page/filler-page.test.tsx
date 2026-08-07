import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor, within } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { RouterHarness } from "@/test/story-utils";
import { FillerPage } from "./filler-page";

// The SHELL's own contract, as distinct from the tabs it hosts.
//
// ⚠ `src/test/filler.test.tsx` already drives this page hard (18 tests: search, the bulk bar,
// split, tag cycling, member gating) and it stays the suite for the Catalog tab's BEHAVIOUR.
// What it does not pin is what the shell owes every tab regardless of which one is showing —
// the two badges and the pool strip — which is exactly what a tab-by-tab extraction can break
// without moving a single snapshot. That gap is what this file covers; duplicating the header
// assertions here would only make two places to update.
//
// ⚠ The counts in the fixtures deliberately DISAGREE: the clip list has 2 items while the watch
// endpoint reports 200. The header must follow the server and the Catalog badge must follow the
// filtered query — equal fixtures could not tell those two rules apart, the same collapsed-
// fixture trap that hid the clip-identity bug in the store.

const jsonResponse = (status: number, body: unknown) =>
  new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });

const clip = (hash: string, name: string) => ({
  hash,
  name,
  kind: "commercial",
  era: 1990,
  audience: "kids",
  category: "toys",
  durationMs: 30_000,
  source: "folder",
  playCount: 0,
  playsCounted: true,
  aiTagged: false,
  tagged: true,
  suggestedEra: 0,
});

const stubFetch = (over: { clips?: unknown[]; incomingTotal?: number } = {}) => {
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string) => {
      const u = String(url);
      if (u.includes("/v1/auth/me")) {
        return Promise.resolve(
          jsonResponse(200, {
            id: "u1",
            name: "Admin",
            role: "admin",
            autoApprove: true,
            disabled: false,
            quota: 0,
          }),
        );
      }
      if (u.includes("/v1/filler/watch")) {
        return Promise.resolve(
          jsonResponse(200, {
            sourcesOn: 4,
            sourcesTotal: 5,
            clips: 200,
            held: 0,
            health: "healthy",
          }),
        );
      }
      if (u.includes("/v1/filler/incoming")) {
        return Promise.resolve(
          jsonResponse(200, {
            asks: [],
            reels: [],
            recentlyFiled: [],
            total: over.incomingTotal ?? 3,
          }),
        );
      }
      if (u.includes("/v1/filler/pool")) {
        return Promise.resolve(jsonResponse(200, { total: 200, untagged: 0, channels: [] }));
      }
      if (u.includes("/v1/filler/sources")) {
        return Promise.resolve(jsonResponse(200, { sources: [], total: 0 }));
      }
      if (u.includes("/v1/settings")) {
        return Promise.resolve(jsonResponse(200, { settings: [], features: { filler: true } }));
      }
      if (u.includes("/v1/filler")) {
        return Promise.resolve(
          jsonResponse(200, { clips: over.clips ?? [clip("hash-a1", "One"), clip("hash-a2", "Two")] }),
        );
      }
      return Promise.resolve(jsonResponse(200, {}));
    }),
  );
};

const makeWrapper = () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
};

const renderPage = (tab: "catalog" | "incoming" | "sources") =>
  render(<RouterHarness content={<FillerPage tab={tab} />} initialPath="/filler" />, {
    wrapper: makeWrapper(),
  });

afterEach(() => vi.unstubAllGlobals());

describe("FillerPage shell", () => {
  // ⚠ The Catalog badge follows the FILTERED query — the opposite rule from the header, on
  // purpose: the badge says "how many am I looking at", the header says "what does the install
  // hold". Moving the clip query into a Catalog tab component would leave this badge empty.
  it("counts the Catalog badge from the filtered clip query, not the server total", async () => {
    stubFetch();
    renderPage("catalog");

    const catalogTab = await screen.findByRole("link", { name: /catalog/i });
    // Two listed clips ⇒ "2", though the watch endpoint reports a 200-clip catalog.
    expect(within(catalogTab).getByText("2")).toBeInTheDocument();
  });

  // ⚠ Admin-only, so this must WAIT for `/v1/auth/me`. Asserting on the link alone would pass
  // instantly against a member's (countless) tab and prove nothing.
  it("counts the Incoming badge from the queue total", async () => {
    stubFetch({ incomingTotal: 7 });
    renderPage("catalog");

    await waitFor(() => {
      const incomingTab = screen.getByRole("link", { name: /incoming/i });
      expect(within(incomingTab).getByText("7")).toBeInTheDocument();
    });
  });

  // ⚠ Sources carries NO count, deliberately: Catalog and Incoming are queues where the number
  // is the reason to look, while the configured-source count is already the header pill's job.
  it("gives the Sources tab no badge", async () => {
    stubFetch();
    renderPage("catalog");

    const sourcesTab = await screen.findByRole("link", { name: /sources/i });
    expect(sourcesTab).toHaveTextContent(/^Sources$/);
  });

  // ⚠ The pool strip is hidden on Sources (the mock's `showPool: fillerTab !== 'sources'`): it
  // answers "can my catalog fill a break", which is context for reading CLIPS. Above the source
  // list it invites the reading that a source is at fault for weak coverage.
  it("shows the pool strip on Catalog", async () => {
    stubFetch();
    renderPage("catalog");

    expect(await screen.findByLabelText("Catalog health")).toBeInTheDocument();
  });

  it("hides the pool strip on Sources", async () => {
    stubFetch();
    renderPage("sources");

    // Wait for the tab to be up, so this cannot pass merely by asserting before the render.
    await screen.findByRole("link", { name: /sources/i });
    expect(screen.queryByLabelText("Catalog health")).not.toBeInTheDocument();
  });
});
