import type { FillerSourcesOutputBody } from "@loomarr/api";
import { getListFillerSourcesMockHandler, getMeMockHandler } from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import type { ReactNode } from "react";
import { describe, expect, it } from "vitest";
import { server } from "@/test/msw/server";
import { SourcesTab } from "./sources-tab";

// SourcesTab is a thin wrapper: it owns the one query the Sources tab needs so the shell no
// longer runs it for a tab that may not be showing. `SourcesPanel` has its own tests for the
// switches, the add form and the per-source search — these cover only the seam.

const ADMIN = {
  local: true,
  id: "u1",
  name: "Admin",
  role: "admin",
  autoApprove: true,
  disabled: false,
  quota: 0,
} as const;

// ⚠ `listed` replaces a `calls: string[]` array that recorded every url and was then searched for
// a substring. Reaching the resolver is a stronger claim than a string match: the handler is bound
// to the generated route, so it cannot fire for the wrong endpoint, and a request to a route with
// NO handler now fails the test outright instead of falling through to a catch-all `{}`.
const stubSources = (body: FillerSourcesOutputBody = { sources: [], total: 0 }) => {
  let listed = false;
  server.use(
    getMeMockHandler({ ...ADMIN }),
    getListFillerSourcesMockHandler(() => {
      listed = true;
      return body;
    }),
  );
  return () => listed;
};

// ⚠ Hand-written: the failure is a STATUS, and this spec declares errors via `default:` (RFC 7807)
// rather than enumerating 5xx, so orval has no code to generate a handler from. A rename still
// fails loudly — the stale path stops matching and the component's real request goes unhandled.
const sourcesFail = () =>
  http.get("*/v1/filler/sources", () =>
    HttpResponse.json({ title: "Boom", detail: "the store is unreachable" }, { status: 500 }),
  );

const makeWrapper = () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
};

describe("SourcesTab", () => {
  it("fetches the source list itself rather than being handed it", async () => {
    const wasListed = stubSources();
    render(<SourcesTab />, { wrapper: makeWrapper() });

    await waitFor(() => expect(wasListed()).toBe(true));
  });

  it("renders a source the server returned", async () => {
    stubSources({
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
          autoAdmit: true,
          admissionControllable: true,
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
    server.use(getMeMockHandler({ ...ADMIN }), sourcesFail());
    render(<SourcesTab />, { wrapper: makeWrapper() });

    expect(await screen.findByText(/add a source/i)).toBeInTheDocument();
  });
});
