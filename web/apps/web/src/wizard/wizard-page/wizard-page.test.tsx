import { getMeMockHandler, getSetupStatusMockHandler } from "@loomarr/api/msw";
import { cleanup, renderUi, screen, userEvent } from "@loomarr/test-support";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createMemoryHistory, createRouter, RouterProvider } from "@tanstack/react-router";
import { HttpResponse, http } from "msw";
import { afterEach, describe, expect, it } from "vitest";
import { routeTree } from "@/routeTree.gen";
import { me } from "@/test/fixtures/users";
import { appHandlers } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";
import { settleOnTeardown } from "@/test/settle-router";

const open = () => {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const router = settleOnTeardown(
    createRouter({
      routeTree,
      context: { queryClient },
      history: createMemoryHistory({ initialEntries: ["/wizard"] }),
    }),
  );
  renderUi(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
};

afterEach(cleanup);

describe("WizardPage", () => {
  it("starts a visitor with no session on the admin account, which cannot be skipped", async () => {
    server.use(
      http.get("*/v1/auth/me", () => HttpResponse.json({ title: "Unauthorized" }, { status: 401 })),
      ...appHandlers(),
    );
    open();

    expect(
      await screen.findByRole("heading", { name: "Create your admin account" }, { timeout: 5000 }),
    ).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /skip/i })).not.toBeInTheDocument();
  });

  // Without the settings the page does not know which steps exist, so it must not guess one.
  it("shows a failed settings read and moves on once a retry succeeds", async () => {
    let down = true;
    server.use(
      getMeMockHandler(me()),
      getSetupStatusMockHandler({ checks: [] }),
      http.get("*/v1/settings", () =>
        down
          ? HttpResponse.json({ title: "Settings unavailable" }, { status: 500 })
          : HttpResponse.json({ settings: [], features: {} }),
      ),
      ...appHandlers(),
    );
    open();
    const user = userEvent.setup();

    expect(await screen.findByText("Settings unavailable", {}, { timeout: 5000 })).toBeInTheDocument();
    down = false;
    await user.click(screen.getByRole("button", { name: /try again|retry/i }));
    expect(await screen.findByRole("button", { name: /back/i }, { timeout: 5000 })).toBeInTheDocument();
    expect(screen.queryByText("Settings unavailable")).not.toBeInTheDocument();
  });
});
