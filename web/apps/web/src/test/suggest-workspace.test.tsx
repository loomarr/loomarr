import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createMemoryHistory, createRouter, RouterProvider } from "@tanstack/react-router";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { routeTree } from "@/routeTree.gen";

// The Suggest workspace end to end through the REAL route tree: intent → submit →
// proposal. The live phases arrive over SSE, which jsdom has no EventSource for, so
// these assert the parts that survive without it — §8's contract is exactly that a
// dropped frame costs a spinner, never a proposal.
const ADMIN = { id: "u1", name: "Ada", role: "admin", autoApprove: true, disabled: false, quota: 0 };
const PROPOSAL = {
  id: "p-1",
  jobId: "job-1",
  status: "submitted",
  proposal: {
    intent: { description: "90s action movies" },
    lineup: [{ mediaType: "movie", tmdbId: 603, name: "The Matrix", year: 1999, inLibrary: true }],
    acquisitions: [],
    rationale: "Grounded against your library.",
  },
};

const json = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });

const stubFetch = (opts: { proposals?: unknown[] } = {}) => {
  const mock = vi.fn((url: string, _init?: RequestInit) => {
    const u = String(url);
    if (u.includes("/v1/auth/me")) return Promise.resolve(json(ADMIN));
    if (u.includes("/v1/suggestions") && String(_init?.method) === "POST") {
      return Promise.resolve(json({ jobId: "job-1" }));
    }
    if (u.includes("/v1/proposals") || u.includes("/v1/suggestions")) {
      return Promise.resolve(json({ proposals: opts.proposals ?? [] }));
    }
    if (u.includes("/v1/settings")) {
      return Promise.resolve(json({ features: {}, settings: [] }));
    }
    return Promise.resolve(json({}));
  });
  vi.stubGlobal("fetch", mock);
  vi.stubGlobal(
    "EventSource",
    class {
      addEventListener() {}
      close() {}
    },
  );
  return mock;
};

const renderAt = (path: string) => {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const router = createRouter({
    routeTree,
    context: { queryClient },
    history: createMemoryHistory({ initialEntries: [path] }),
  });
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
};

afterEach(() => vi.restoreAllMocks());

describe("Suggest workspace", () => {
  it("submits the typed intent, including the constraints", async () => {
    const fetchMock = stubFetch();
    renderAt("/suggest");

    await userEvent.type(await screen.findByLabelText("Channel intent"), "90s action movies");
    await userEvent.click(screen.getByRole("button", { name: /add constraints/i }));
    await userEvent.type(screen.getByLabelText(/target runtime/i), "180");
    await userEvent.click(screen.getByRole("button", { name: /suggest a lineup/i }));

    const posted = fetchMock.mock.calls.find(
      ([u, init]) => String(u).includes("/v1/suggestions") && String(init?.method) === "POST",
    );
    expect(posted).toBeTruthy();
    // The wire's field names — runtimeTargetMin was unreachable until 13.4a's prerequisite.
    expect(JSON.parse(String(posted?.[1]?.body))).toMatchObject({
      description: "90s action movies",
      runtimeTargetMin: 180,
    });
  });

  it("prefills the intent the wizard handed off", async () => {
    stubFetch();
    renderAt("/suggest?intent=saturday-morning%20cartoons");
    expect(await screen.findByLabelText("Channel intent")).toHaveValue("saturday-morning cartoons");
  });

  it("shows the grounded proposal once the run produces one", async () => {
    stubFetch({ proposals: [PROPOSAL] });
    renderAt("/suggest");

    await userEvent.type(await screen.findByLabelText("Channel intent"), "90s action movies");
    await userEvent.click(screen.getByRole("button", { name: /suggest a lineup/i }));

    expect(await screen.findByText("The Matrix")).toBeInTheDocument();
  });
});
