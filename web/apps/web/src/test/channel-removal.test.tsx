import { getDeleteChannelMockHandler, getGetChannelMockHandler, getMeMockHandler } from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createMemoryHistory, createRouter, RouterProvider } from "@tanstack/react-router";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { toast } from "sonner";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { routeTree } from "@/routeTree.gen";
import { channel } from "@/test/fixtures/channels";
import { me } from "@/test/fixtures/users";
import { appHandlers } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";

vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

// Route-level coverage for the result a person sees after each DELETE mode. The component test
// pins the payload; this one drives the real route through the generated client and renders the
// toast, so a truthful button cannot regress to the old one-size-fits-all "Channel deleted" result.
const renderDanger = () => {
  const deletes: string[] = [];
  server.use(
    getMeMockHandler(me()),
    getGetChannelMockHandler(channel({ id: "ch-1", name: "90s Action" })),
    getDeleteChannelMockHandler(({ request }) => {
      deletes.push(request.url);
      return undefined as never;
    }),
    ...appHandlers(),
  );

  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const router = createRouter({
    routeTree,
    context: { queryClient },
    history: createMemoryHistory({ initialEntries: ["/channels/ch-1/danger"] }),
  });
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
  return { deletes };
};

describe("channel removal feedback", () => {
  beforeEach(() => vi.clearAllMocks());

  it("reports that stop-managing kept Loomarr and Tunarr records", async () => {
    const user = userEvent.setup();
    const { deletes } = renderDanger();

    await user.click(await screen.findByRole("button", { name: "Stop managing" }));
    await user.click(screen.getByRole("button", { name: "Stop managing" }));

    await waitFor(() => expect(deletes).toHaveLength(1));
    expect(deletes[0]).toContain("purge=false");
    expect(toast.success).toHaveBeenCalledWith("Stopped managing channel", {
      description: "Loomarr kept its record and left any Tunarr channel in place.",
    });
  });

  it("reports that purge removed Loomarr and Tunarr records", async () => {
    const user = userEvent.setup();
    const { deletes } = renderDanger();

    await user.click(await screen.findByRole("button", { name: "Delete from Loomarr and Tunarr" }));
    await user.click(screen.getByRole("button", { name: "Delete from Loomarr and Tunarr" }));

    await waitFor(() => expect(deletes).toHaveLength(1));
    expect(deletes[0]).toContain("purge=true");
    expect(toast.success).toHaveBeenCalledWith("Channel deleted from Loomarr and Tunarr", {
      description: "Its Loomarr record and any retained Tunarr channel were removed.",
    });
  });
});
