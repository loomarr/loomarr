import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { SourcesTab } from "./sources-tab";

// SourcesTab is a thin wrapper: it owns the one query the Sources tab needs so the shell no
// longer runs it for a tab that may not be showing. `SourcesPanel` has its own tests for the
// switches, the add form and the per-source search — these cover only the seam.

const jsonResponse = (status: number, body: unknown) =>
  new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });

const stubFetch = (sourcesStatus = 200, body: unknown = { sources: [], total: 0 }) => {
  const calls: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string) => {
      const u = String(url);
      calls.push(u);
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
      if (u.includes("/v1/filler/sources")) {
        return Promise.resolve(jsonResponse(sourcesStatus, body));
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

afterEach(() => vi.unstubAllGlobals());

describe("SourcesTab", () => {
  it("fetches the source list itself rather than being handed it", async () => {
    const calls = stubFetch();
    render(<SourcesTab />, { wrapper: makeWrapper() });

    await waitFor(() => expect(calls.some((u) => u.includes("/v1/filler/sources"))).toBe(true));
  });

  it("renders a source the server returned", async () => {
    stubFetch(200, {
      sources: [
        {
          id: "folder",
          kind: "folder",
          target: "/data/filler",
          detail: "watched directly",
          count: 12,
          configured: true,
          fetchable: false,
          enabled: true,
          switchable: true,
          removable: false,
          searchable: false,
        },
      ],
      total: 12,
    });
    render(<SourcesTab />, { wrapper: makeWrapper() });

    expect(await screen.findByText("/data/filler")).toBeInTheDocument();
  });

  // ⚠ A failed list must still render the tab, not blank it. Sources is where an operator goes
  // to find out WHY filler is broken, so its own error is the least useful moment to lose the
  // screen. The panel threads `sourcesError` into the source list's error slot (it renders
  // beside the rows), so what this seam guarantees is that a 500 does not throw and the
  // "Add a source" affordance survives — the operator can still act.
  it("stays renderable when the source list fails", async () => {
    stubFetch(500, { title: "Boom", detail: "the store is unreachable" });
    render(<SourcesTab />, { wrapper: makeWrapper() });

    expect(await screen.findByText(/add a source/i)).toBeInTheDocument();
  });
});
