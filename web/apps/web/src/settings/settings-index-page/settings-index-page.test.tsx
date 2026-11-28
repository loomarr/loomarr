import { getMeMockHandler } from "@loomarr/api/msw";
import { cleanup, renderUi, screen, userEvent } from "@loomarr/test-support";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createMemoryHistory, createRouter, RouterProvider } from "@tanstack/react-router";
import { afterEach, describe, expect, it } from "vitest";
import { routeTree } from "@/routeTree.gen";
import { me } from "@/test/fixtures/users";
import { appHandlers } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";
import { settleOnTeardown } from "@/test/settle-router";

const open = (role: "admin" | "member") => {
  server.use(getMeMockHandler(me({ role })), ...appHandlers());
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const router = settleOnTeardown(
    createRouter({
      routeTree,
      context: { queryClient },
      history: createMemoryHistory({ initialEntries: ["/settings"] }),
    }),
  );
  renderUi(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
};

const finder = () => screen.findByRole("searchbox", { name: "Find a setting" }, { timeout: 5000 });

afterEach(cleanup);

describe("SettingsIndexPage", () => {
  it("says so when a search matches nothing", async () => {
    open("admin");
    const user = userEvent.setup();

    await user.type(await finder(), "zzzzqqqq");

    expect(await screen.findByText("No settings found")).toBeInTheDocument();
  });

  it("finds an admin's setting by name and keeps it from a member", async () => {
    const user = userEvent.setup();
    open("admin");
    await user.type(await finder(), "backup");
    expect(await screen.findByRole("navigation", { name: "Settings search results" })).toHaveTextContent(
      /backup/i,
    );
    cleanup();

    open("member");
    await user.type(await finder(), "backup");
    expect(await screen.findByText("No settings found")).toBeInTheDocument();
  });
});
