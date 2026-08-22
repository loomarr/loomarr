import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  RouterProvider,
} from "@tanstack/react-router";
import { render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { MyRequests } from "./my-requests";

const jsonResponse = (body: unknown) =>
  new Response(JSON.stringify(body), { status: 200, headers: { "content-type": "application/json" } });

const journey = (over: Record<string, unknown> = {}) => ({
  version: 1,
  jobId: "j1",
  milestone: "generating",
  intent: { description: "90s action night" },
  attempts: [],
  actions: ["wait"],
  createdAt: "2026-08-22T10:00:00Z",
  updatedAt: "2026-08-22T10:00:00Z",
  ...over,
});

const renderRequests = () => {
  const rootRoute = createRootRoute();
  const route = createRoute({ getParentRoute: () => rootRoute, path: "/", component: MyRequests });
  const router = createRouter({ routeTree: rootRoute.addChildren([route]), history: createMemoryHistory() });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
};

const stubJourneys = (journeys: unknown[]) => {
  const urls: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string) => {
      if (typeof url === "string" && url.includes("/v1/proposal-jobs")) urls.push(url);
      return Promise.resolve(jsonResponse({ journeys }));
    }),
  );
  return urls;
};

afterEach(() => vi.unstubAllGlobals());

describe("MyRequests", () => {
  it("restores an in-flight request that has no Proposal", async () => {
    stubJourneys([journey()]);
    renderRequests();
    expect(await screen.findByText("My requests")).toBeInTheDocument();
    expect(screen.getByText("90s action night")).toBeInTheDocument();
    expect(screen.getByText("Generating")).toBeInTheDocument();
  });

  it("asks the server to scope the list to the caller", async () => {
    const urls = stubJourneys([journey()]);
    renderRequests();
    await screen.findByText("My requests");
    expect(urls).toHaveLength(1);
    expect(urls[0]).toContain("mine=true");
  });

  it("shows safe failure recovery from server-authorized actions", async () => {
    stubJourneys([
      journey({
        milestone: "failed",
        failure: { code: "no_grounded_titles", message: "No grounded titles matched this request." },
        actions: ["edit", "retry"],
      }),
    ]);
    renderRequests();
    expect(await screen.findByText("Needs attention")).toBeInTheDocument();
    expect(screen.getByText("No grounded titles matched this request.")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Edit and try again" })).toHaveAttribute(
      "href",
      "/guide?intent=90s+action+night",
    );
  });

  it("renders nothing when the member has no requests", async () => {
    stubJourneys([]);
    const { container } = renderRequests();
    await waitFor(() => expect(container).toBeEmptyDOMElement());
  });
});
