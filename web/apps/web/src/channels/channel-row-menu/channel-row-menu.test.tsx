import { getDeleteChannelMockHandler, getUpdateChannelMockHandler } from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";
import { channel } from "@/test/fixtures/channels";
import { server } from "@/test/msw/server";
import { ChannelRowMenu } from "./channel-row-menu";

const makeWrapper = () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
};

// Records the PATCH body and the DELETE url so a test can prove pause sends status:paused and
// delete carries purge=true.
//
// ⚠ The stub this replaced recorded `method + url + body` for EVERY request and the assertions then
// filtered by method — `calls.find((c) => c.method === "PATCH")`. That filter is the weak part: it
// would have matched a PATCH to any endpoint at all. These handlers are bound to
// `*/v1/channels/:id`, so being in the resolver already proves the route, and only the payload
// still needs recording.
const stubChannel = () => {
  const patches: unknown[] = [];
  const deletes: string[] = [];
  server.use(
    getUpdateChannelMockHandler(async ({ request }) => {
      patches.push(await request.json());
      return channel();
    }),
    getDeleteChannelMockHandler(({ request }) => {
      deletes.push(request.url);
      return undefined as never;
    }),
  );
  return { patches, deletes };
};

const live = { id: "ch-1", name: "Late Night Noir", status: "live" as const };
const paused = { ...live, status: "paused" as const };

describe("ChannelRowMenu", () => {
  it("opening the menu does not navigate (the row is a Link; the menu swallows its clicks)", async () => {
    const user = userEvent.setup();
    stubChannel();
    // The menu is rendered inside a real anchor to mirror the row: a click that leaked to the
    // Link would fire this handler. It must NOT.
    const onLinkClick = vi.fn((e: React.MouseEvent) => e.preventDefault());
    render(
      // A real anchor standing in for the row Link, to prove the menu's clicks don't leak to it.
      <a href="/channels/ch-1" onClick={onLinkClick}>
        <ChannelRowMenu channel={live} />
      </a>,
      { wrapper: makeWrapper() },
    );

    await user.click(screen.getByRole("button", { name: /actions for/i }));
    // `findBy`: the popup is portalled and mounts asynchronously (V50b — Base UI Menu).
    expect(await screen.findByRole("menu")).toBeInTheDocument();
    expect(onLinkClick).not.toHaveBeenCalled();
  });

  it("Pause sends a PATCH with status:paused", async () => {
    const user = userEvent.setup();
    const { patches } = stubChannel();
    render(<ChannelRowMenu channel={live} />, { wrapper: makeWrapper() });

    await user.click(screen.getByRole("button", { name: /actions for/i }));
    await user.click(await screen.findByRole("menuitem", { name: "Pause" }));

    await waitFor(() => expect(patches).toHaveLength(1));
    expect(patches[0]).toMatchObject({ status: "paused" });
  });

  it("a paused channel offers Resume (status:building)", async () => {
    const user = userEvent.setup();
    const { patches } = stubChannel();
    render(<ChannelRowMenu channel={paused} />, { wrapper: makeWrapper() });

    await user.click(screen.getByRole("button", { name: /actions for/i }));
    await user.click(await screen.findByRole("menuitem", { name: "Resume" }));

    await waitFor(() => expect(patches).toHaveLength(1));
    expect(patches[0]).toMatchObject({ status: "building" });
  });

  it("Delete is a two-step confirm (no name typing) and DELETEs with purge=true", async () => {
    const user = userEvent.setup();
    const { deletes } = stubChannel();
    render(<ChannelRowMenu channel={live} />, { wrapper: makeWrapper() });

    await user.click(screen.getByRole("button", { name: /actions for/i }));
    await user.click(await screen.findByRole("menuitem", { name: /delete/i }));

    // Step 2 is a plain confirm — no textbox to fill, the execute button is enabled.
    expect(screen.queryByRole("textbox")).not.toBeInTheDocument();
    const confirm = screen.getByRole("button", { name: "Delete" });
    expect(confirm).toBeEnabled();
    await user.click(confirm);

    await waitFor(() => expect(deletes).toHaveLength(1));
    // Purge — a list delete fully removes the channel (the maintainer's choice), so the URL
    // carries purge=true.
    expect(deletes[0]).toContain("purge=true");
  });
});
