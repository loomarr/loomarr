import {
  getListUserSessionsMockHandler,
  getMeMockHandler,
  getRevokeSessionMockHandler,
} from "@loomarr/api/msw";
import { cleanup, renderUi, screen, userEvent, waitFor } from "@loomarr/test-support";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createMemoryHistory, createRouter, RouterProvider } from "@tanstack/react-router";
import { afterEach, describe, expect, it } from "vitest";
import { routeTree } from "@/routeTree.gen";
import { me } from "@/test/fixtures/users";
import { appHandlers } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";
import { settleOnTeardown } from "@/test/settle-router";

// The page sits in the app shell and reads the signed-in user, so it is mounted at its address.
const open = (local: boolean) => {
  const revoked: string[] = [];
  server.use(
    getMeMockHandler(me({ role: "member", local })),
    getListUserSessionsMockHandler({
      sessions: [
        { id: "mine", createdAt: Date.UTC(2026, 0, 1), current: true, clientLabel: "Firefox on Linux" },
        { id: "other", createdAt: Date.UTC(2026, 0, 1), current: false, clientLabel: "Safari on iOS" },
      ],
    }),
    getRevokeSessionMockHandler(({ params }) => {
      revoked.push(String(params.hash));
    }),
    ...appHandlers(),
  );
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const router = settleOnTeardown(
    createRouter({
      routeTree,
      context: { queryClient },
      history: createMemoryHistory({ initialEntries: ["/account"] }),
    }),
  );
  renderUi(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
  return revoked;
};

afterEach(cleanup);

describe("AccountPage", () => {
  it("signs another session out only after the confirm", async () => {
    const revoked = open(true);
    const user = userEvent.setup();
    await screen.findByText("Safari on iOS", {}, { timeout: 5000 });

    // Only the other device offers it: your own session has no Sign out in the list.
    const row = screen.getByText("Safari on iOS").closest("li") as HTMLElement;
    await user.click(row.querySelector("button") as HTMLButtonElement);
    expect(screen.getByText(/Sign out Safari on iOS\?/)).toBeInTheDocument();
    expect(revoked).toEqual([]);

    const confirm = screen.getByText(/Sign out Safari on iOS\?/).closest("li") as HTMLElement;
    await user.click(
      [...confirm.querySelectorAll("button")].find((b) => b.textContent === "Sign out") as HTMLButtonElement,
    );
    await waitFor(() => expect(revoked).toEqual(["other"]));
  });

  it("offers a password change to a local account only", async () => {
    open(false);

    expect(
      await screen.findByText(/Your media server owns this password/, {}, { timeout: 5000 }),
    ).toBeInTheDocument();
    expect(screen.queryByLabelText("New password")).not.toBeInTheDocument();
  });
});
