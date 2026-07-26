import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render as rtlRender, screen, waitFor } from "@testing-library/react";
import type { ReactElement, ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { MyRequests } from "./my-requests";

const makeWrapper = () => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
};

const render = (ui: ReactElement) => rtlRender(ui, { wrapper: makeWrapper() });

const jsonResponse = (body: unknown) =>
  new Response(JSON.stringify(body), { status: 200, headers: { "content-type": "application/json" } });

// Answers per status, and records every requested URL so a test can assert the SCOPING
// parameter actually goes out — the whole point of the feature is that this list is mine.
const stubProposals = (byStatus: Record<string, unknown[]>) => {
  const urls: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string) => {
      if (typeof url === "string" && url.includes("/v1/suggestions")) {
        urls.push(url);
        const status = new URL(url, "http://x").searchParams.get("status") ?? "";
        return Promise.resolve(jsonResponse({ proposals: byStatus[status] ?? [] }));
      }
      return Promise.resolve(jsonResponse({}));
    }),
  );
  return urls;
};

afterEach(() => vi.unstubAllGlobals());

const proposal = (over: Record<string, unknown> = {}) => ({
  id: "p1",
  jobId: "j1",
  status: "submitted",
  proposal: { intent: { description: "90s action night" }, lineup: [], acquisitions: [] },
  ...over,
});

describe("MyRequests", () => {
  // The reachability assertion the phase gate asks for. `A2`'s defect was that a member could
  // submit a request and see NOTHING — the queue page never queried /v1/suggestions at all.
  it("mounts and renders the member's requests", async () => {
    stubProposals({ submitted: [proposal()] });
    render(<MyRequests />);

    expect(await screen.findByText("My requests")).toBeInTheDocument();
    expect(screen.getByText("90s action night")).toBeInTheDocument();
  });

  // ⚠ Scoping is the feature. Without `mine=true` this renders every member's requests — which
  // is the unscoped approval queue, on a page headed "My requests".
  it("asks the server to scope the list to the caller", async () => {
    const urls = stubProposals({ submitted: [proposal()] });
    render(<MyRequests />);

    await screen.findByText("My requests");
    expect(urls.length).toBeGreaterThan(0);
    for (const u of urls) {
      expect(u).toContain("mine=true");
    }
  });

  // A member's question is "what happened to what I asked for?", and the two answers that
  // matter most live outside `submitted`. The endpoint filters by one status per call.
  it("covers submitted, approved and denied", async () => {
    const urls = stubProposals({
      submitted: [proposal()],
      approved: [proposal({ id: "p2", status: "approved" })],
      denied: [proposal({ id: "p3", status: "denied", denyReason: "over the cap" })],
    });
    render(<MyRequests />);

    await waitFor(() => expect(screen.getAllByText("90s action night")).toHaveLength(3));
    const statuses = urls.map((u) => new URL(u, "http://x").searchParams.get("status"));
    expect(new Set(statuses)).toEqual(new Set(["submitted", "approved", "denied"]));
  });

  // The tracked-titles table below is the page's real content; an "you have asked for nothing"
  // panel above it would be noise on the common path.
  it("renders nothing when the member has no requests", async () => {
    stubProposals({});
    const { container } = render(<MyRequests />);

    await waitFor(() => expect(container).toBeEmptyDOMElement());
  });
});
