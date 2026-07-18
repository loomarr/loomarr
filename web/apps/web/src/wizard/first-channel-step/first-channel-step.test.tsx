import { CHANNEL_TEMPLATES } from "@loomarr/core";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createMemoryHistory, createRootRoute, createRouter, RouterProvider } from "@tanstack/react-router";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { FirstChannelStep } from "./first-channel-step";

const json = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });

const stubFetch = () => {
  const mock = vi.fn((_url: string, _init?: RequestInit) => Promise.resolve(json({ results: [] })));
  vi.stubGlobal("fetch", mock);
  return mock;
};

// useCompleteSetup navigates via the router, so the step needs one mounted.
const renderStep = () => {
  const rootRoute = createRootRoute({ component: () => <FirstChannelStep /> });
  const router = createRouter({
    routeTree: rootRoute,
    history: createMemoryHistory({ initialEntries: ["/"] }),
  });
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
  return router;
};

afterEach(() => vi.restoreAllMocks());

describe("FirstChannelStep", () => {
  it("offers the starter templates §13 names", async () => {
    stubFetch();
    renderStep();
    for (const t of CHANNEL_TEMPLATES) {
      expect(await screen.findByText(t.label)).toBeInTheDocument();
    }
  });

  it("picking a template completes setup and hands off to Suggest with the intent", async () => {
    const fetchMock = stubFetch();
    const router = renderStep();

    const first = CHANNEL_TEMPLATES[0];
    await userEvent.click(await screen.findByText(String(first?.label)));

    // Completing the wizard is what flips `setup.completed`, so `/` stops routing here.
    await waitFor(() => {
      const patch = fetchMock.mock.calls.find(([u]) => String(u).includes("/v1/settings"));
      expect(patch).toBeTruthy();
      expect(JSON.parse(String(patch?.[1]?.body))).toEqual({ edits: { "setup.completed": "true" } });
    });
    await waitFor(() => {
      expect(router.history.location.pathname).toBe("/suggest");
      expect(decodeURIComponent(router.history.location.search)).toContain(String(first?.description));
    });
  });

  it("finishing without a channel still completes setup", async () => {
    const fetchMock = stubFetch();
    const router = renderStep();

    await userEvent.click(await screen.findByRole("button", { name: /finish setup without a channel/i }));
    await waitFor(() => {
      expect(fetchMock.mock.calls.some(([u]) => String(u).includes("/v1/settings"))).toBe(true);
      expect(router.history.location.pathname).toBe("/channels");
    });
  });
});
