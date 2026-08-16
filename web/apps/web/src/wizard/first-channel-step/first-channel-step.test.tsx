import { getSettingsPatchMockHandler } from "@loomarr/api/msw";
import { CHANNEL_TEMPLATES } from "@loomarr/core";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createMemoryHistory, createRootRoute, createRouter, RouterProvider } from "@tanstack/react-router";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { server } from "@/test/msw/server";
import { FirstChannelStep } from "./first-channel-step";

// ⚠ The old stub answered EVERY url with `{ results: [] }`, and the assertions then searched
// `fetchMock.mock.calls` for one containing "/v1/settings". Two consequences worth naming, because
// they are why this migration is not cosmetic:
//
//   • A catch-all cannot fail. Had the step called some other endpoint — or called nothing at all
//     and the navigation come from elsewhere — the stub would have answered happily either way.
//   • The assertion matched a url SUBSTRING the test wrote itself, so it proved the test's own
//     spelling, not the route. Binding the handler to the generated route makes arriving in the
//     resolver the proof.
const stubSettings = () => {
  const patches: unknown[] = [];
  server.use(
    getSettingsPatchMockHandler(async ({ request }) => {
      patches.push(await request.json());
      return { results: [] };
    }),
  );
  return { patches };
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

describe("FirstChannelStep", () => {
  it("offers the starter templates §13 names", async () => {
    stubSettings();
    renderStep();
    for (const t of CHANNEL_TEMPLATES) {
      expect(await screen.findByText(t.label)).toBeInTheDocument();
    }
  });

  it("picking a template completes setup and hands off its stable id to the Guide", async () => {
    const { patches } = stubSettings();
    const router = renderStep();

    const first = CHANNEL_TEMPLATES[0];
    await userEvent.click(await screen.findByText(String(first?.label)));

    // Completing the wizard is what flips `setup.completed`, so `/` stops routing here.
    await waitFor(() => expect(patches).toEqual([{ edits: { "setup.completed": "true" } }]));
    await waitFor(() => {
      expect(router.history.location.pathname).toBe("/guide");
      expect(router.history.location.search).toBe(`?preset=${first?.id}`);
    });
  });

  it("finishing without a channel still completes setup", async () => {
    const { patches } = stubSettings();
    const router = renderStep();

    await userEvent.click(await screen.findByRole("button", { name: /finish setup without a channel/i }));
    await waitFor(() => {
      expect(patches).toHaveLength(1);
      expect(router.history.location.pathname).toBe("/guide");
    });
  });
});
