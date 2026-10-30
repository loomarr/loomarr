import type { MeBody } from "@loomarr/api";
import { LoomarrProvider } from "@loomarr/design-system";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createMemoryHistory, createRouter, RouterProvider } from "@tanstack/react-router";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ChannelWatchProps } from "@/channels/channel-watch";
import { routeTree } from "@/routeTree.gen";
import { appHandlers } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";

// Watch is drawn by the authed layout, not by `/watch/$id` (#2088 decision 4), so a channel in the
// browser's picture-in-picture keeps playing after the viewer leaves. The player itself is stood in
// for: jsdom has no media pipeline and no picture-in-picture, and ChannelWatch is tested on its own.
// What is pinned here is the mounting — through the REAL route tree, since that is where it lives.
const mounts = vi.fn();
vi.mock("@/channels/channel-watch", async () => {
  const { useEffect } = await import("react");
  const ChannelWatch = ({ channel, onLeave, onPictureInPicture }: ChannelWatchProps) => {
    useEffect(() => {
      mounts();
    }, []);
    return (
      <div>
        <span>Playing {channel.name}</span>
        <button type="button" onClick={() => onPictureInPicture?.(true)}>
          Enter picture-in-picture
        </button>
        <button type="button" onClick={() => onPictureInPicture?.(false)}>
          Close picture-in-picture
        </button>
        <button type="button" onClick={onLeave}>
          Leave Watch
        </button>
      </div>
    );
  };
  return { ChannelWatch };
});

const MEMBER: MeBody = {
  autoApprove: false,
  disabled: false,
  id: "u2",
  local: true,
  name: "Bo",
  offlineLogin: false,
  quota: 0,
  role: "member",
};

const clients: QueryClient[] = [];
afterEach(async () => {
  mounts.mockReset();
  await Promise.all(clients.splice(0).map((c) => vi.waitFor(() => expect(c.isFetching()).toBe(0))));
});

const renderApp = (initialPath: string) => {
  server.use(
    ...appHandlers(),
    http.get("*/v1/auth/me", () => HttpResponse.json(MEMBER)),
  );
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  clients.push(queryClient);
  const router = createRouter({
    context: { queryClient },
    history: createMemoryHistory({ initialEntries: [initialPath] }),
    routeTree,
  });
  render(
    <QueryClientProvider client={queryClient}>
      <LoomarrProvider theme="dark">
        <RouterProvider router={router} />
      </LoomarrProvider>
    </QueryClientProvider>,
  );
  return router;
};

describe("Watch in the authed layout", () => {
  it("plays the route's channel full-bleed and names the document after it", async () => {
    renderApp("/watch/ch-1");

    expect(await screen.findByText("Playing Late Night Noir")).toBeVisible();
    await waitFor(() => expect(document.title).toContain("Late Night Noir"));
    // Full-bleed: no app navigation around the picture.
    expect(screen.queryByRole("navigation", { name: /main|primary/i })).not.toBeInTheDocument();
  });

  it("leaves to the Guide from a deep link with no history", async () => {
    const router = renderApp("/watch/ch-1");

    await userEvent.click(await screen.findByRole("button", { name: "Leave Watch" }));
    await waitFor(() => expect(router.state.location.pathname).toBe("/guide"));
    await waitFor(() => expect(screen.queryByText("Playing Late Night Noir")).not.toBeInTheDocument());
  });

  it("keeps the same player mounted, hidden, while picture-in-picture plays off /watch, until it closes", async () => {
    const router = renderApp("/watch/ch-1");
    await userEvent.click(await screen.findByRole("button", { name: "Enter picture-in-picture" }));

    await router.navigate({ to: "/guide" });
    await waitFor(() => expect(router.state.location.pathname).toBe("/guide"));
    // Still there — the same instance, not a remount — but out of sight and out of the page's title.
    const playing = screen.getByText("Playing Late Night Noir", { selector: "span" });
    expect(playing).not.toBeVisible();
    expect(mounts).toHaveBeenCalledOnce();
    expect(document.title).not.toContain("Late Night Noir");

    // The browser's floating window closed: Watch goes, and the channel stops.
    await userEvent.click(screen.getByRole("button", { name: "Close picture-in-picture", hidden: true }));
    expect(screen.queryByText("Playing Late Night Noir")).not.toBeInTheDocument();
  });
});
