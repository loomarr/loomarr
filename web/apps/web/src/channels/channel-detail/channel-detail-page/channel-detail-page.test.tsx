import { getChannelsNowNextMockHandler, getGetChannelMockHandler, getMeMockHandler } from "@loomarr/api/msw";
import { cleanup, renderUi, screen } from "@loomarr/test-support";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createMemoryHistory, createRouter, RouterProvider } from "@tanstack/react-router";
import { afterEach, describe, expect, it } from "vitest";
import { routeTree } from "@/routeTree.gen";
import { channel } from "@/test/fixtures/channels";
import { me } from "@/test/fixtures/users";
import { appHandlers } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";
import { settleOnTeardown } from "@/test/settle-router";

// The page reads the router's path and links, so it is mounted at its own address.
const open = (role: "admin" | "member") => {
  server.use(
    getMeMockHandler(me({ role })),
    // Before the channel read, whose `/v1/channels/:id` path would otherwise answer it.
    getChannelsNowNextMockHandler({ channels: [] }),
    getGetChannelMockHandler(channel({ status: "paused", number: 7 })),
    ...appHandlers(),
  );
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const router = settleOnTeardown(
    createRouter({
      routeTree,
      context: { queryClient },
      history: createMemoryHistory({ initialEntries: ["/channels/ch-1/info"] }),
    }),
  );
  renderUi(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
};

afterEach(cleanup);

describe("ChannelDetailPage", () => {
  it("shows a member the channel and its air line, with no editors or section bar", async () => {
    open("member");

    expect(
      await screen.findByRole("heading", { level: 1, name: "Late Night Noir" }, { timeout: 5000 }),
    ).toBeInTheDocument();
    expect(screen.getAllByText("Paused").length).toBeGreaterThan(0);
    expect(screen.queryByRole("textbox", { name: "Channel name" })).not.toBeInTheDocument();
    expect(screen.queryByTestId("channel-tab-strip")).not.toBeInTheDocument();
  });

  it("gives an admin the section bar", async () => {
    open("admin");

    expect(await screen.findByTestId("channel-tab-strip", {}, { timeout: 5000 })).toHaveTextContent(
      "Danger zone",
    );
  });
});
