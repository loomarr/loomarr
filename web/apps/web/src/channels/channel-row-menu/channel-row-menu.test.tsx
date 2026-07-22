import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ChannelRowMenu } from "./channel-row-menu";

const makeWrapper = () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
};

const jsonResponse = (status: number, body: unknown) =>
  new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });

// Records PATCH/DELETE method + url + parsed body so a test can prove pause sends
// status:paused and delete sends purge=true.
const stubFetch = () => {
  const calls: { method: string; url: string; body: unknown }[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string, init?: RequestInit) => {
      const method = init?.method ?? "GET";
      calls.push({
        method,
        url: String(url),
        body: init?.body ? JSON.parse(init.body as string) : undefined,
      });
      return Promise.resolve(jsonResponse(method === "DELETE" ? 204 : 200, { id: "ch-1" }));
    }),
  );
  return { calls };
};

const live = { id: "ch-1", name: "Late Night Noir", status: "live" as const };
const paused = { ...live, status: "paused" as const };

afterEach(() => vi.restoreAllMocks());

describe("ChannelRowMenu", () => {
  it("opening the menu does not navigate (the row is a Link; the menu swallows its clicks)", async () => {
    const user = userEvent.setup();
    stubFetch();
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
    expect(screen.getByRole("menu")).toBeInTheDocument();
    expect(onLinkClick).not.toHaveBeenCalled();
  });

  it("Pause sends a PATCH with status:paused", async () => {
    const user = userEvent.setup();
    const { calls } = stubFetch();
    render(<ChannelRowMenu channel={live} />, { wrapper: makeWrapper() });

    await user.click(screen.getByRole("button", { name: /actions for/i }));
    await user.click(screen.getByRole("menuitem", { name: "Pause" }));

    await waitFor(() => expect(calls.some((c) => c.method === "PATCH")).toBe(true));
    const patch = calls.find((c) => c.method === "PATCH");
    expect(patch?.body).toMatchObject({ status: "paused" });
  });

  it("a paused channel offers Resume (status:building)", async () => {
    const user = userEvent.setup();
    const { calls } = stubFetch();
    render(<ChannelRowMenu channel={paused} />, { wrapper: makeWrapper() });

    await user.click(screen.getByRole("button", { name: /actions for/i }));
    await user.click(screen.getByRole("menuitem", { name: "Resume" }));

    await waitFor(() => expect(calls.some((c) => c.method === "PATCH")).toBe(true));
    expect(calls.find((c) => c.method === "PATCH")?.body).toMatchObject({ status: "building" });
  });

  it("Delete is typed-confirm gated and DELETEs with purge=true", async () => {
    const user = userEvent.setup();
    const { calls } = stubFetch();
    render(<ChannelRowMenu channel={live} />, { wrapper: makeWrapper() });

    await user.click(screen.getByRole("button", { name: /actions for/i }));
    await user.click(screen.getByRole("menuitem", { name: /delete/i }));

    // The confirm Delete is disabled until the exact name is typed.
    const confirm = screen.getByRole("button", { name: "Delete" });
    expect(confirm).toBeDisabled();
    await user.type(screen.getByLabelText(/type "late night noir" to delete/i), "Late Night Noir");
    expect(confirm).toBeEnabled();
    await user.click(confirm);

    await waitFor(() => expect(calls.some((c) => c.method === "DELETE")).toBe(true));
    const del = calls.find((c) => c.method === "DELETE");
    // Purge — a list delete fully removes the channel (the maintainer's choice), so the URL
    // carries purge=true.
    expect(del?.url).toContain("purge=true");
  });
});
