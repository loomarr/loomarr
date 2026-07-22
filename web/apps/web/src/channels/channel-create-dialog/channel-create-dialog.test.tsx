import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ChannelCreateDialog } from "./channel-create-dialog";

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

// Captures the create POST body so a test can prove it sends NO id (the server assigns one)
// and returns a server-minted id the dialog then hands back for navigation.
const stubFetch = (opts: { status?: number; body?: unknown } = {}) => {
  const posts: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn((_url: string, init?: RequestInit) => {
      if ((init?.method ?? "GET") === "POST") {
        posts.push(init?.body ? JSON.parse(init.body as string) : undefined);
        return Promise.resolve(jsonResponse(opts.status ?? 200, opts.body ?? { id: "ch_server123" }));
      }
      return Promise.resolve(jsonResponse(200, {}));
    }),
  );
  return { posts };
};

afterEach(() => vi.restoreAllMocks());

describe("ChannelCreateDialog", () => {
  it("keeps Create disabled until a name and a valid number are entered", async () => {
    const user = userEvent.setup();
    stubFetch();
    render(<ChannelCreateDialog onCreated={() => {}} onClose={() => {}} />, { wrapper: makeWrapper() });

    const create = screen.getByRole("button", { name: /create channel/i });
    expect(create).toBeDisabled();

    await user.type(screen.getByLabelText("Name"), "Late Night Noir");
    expect(create).toBeDisabled(); // number still missing
    await user.type(screen.getByLabelText("Number"), "7");
    expect(create).toBeEnabled();
  });

  it("posts name/number/strategy with NO id, then hands the server-assigned id to onCreated", async () => {
    const user = userEvent.setup();
    const { posts } = stubFetch({ body: { id: "ch_server123" } });
    const onCreated = vi.fn();
    render(<ChannelCreateDialog onCreated={onCreated} onClose={() => {}} />, { wrapper: makeWrapper() });

    await user.type(screen.getByLabelText("Name"), "Late Night Noir");
    await user.type(screen.getByLabelText("Number"), "7");
    await user.click(screen.getByRole("button", { name: /create channel/i }));

    await waitFor(() => expect(onCreated).toHaveBeenCalledWith("ch_server123"));
    expect(posts).toHaveLength(1);
    const body = posts[0] as Record<string, unknown>;
    expect(body).toMatchObject({ name: "Late Night Noir", number: 7, strategy: "sequential" });
    // The whole point of the server-assigned-id change: the client sends no id.
    expect("id" in body).toBe(false);
  });

  it("surfaces a duplicate-number 409 inline and does not navigate", async () => {
    const user = userEvent.setup();
    stubFetch({ status: 409, body: { title: "channel number already in use", status: 409 } });
    const onCreated = vi.fn();
    render(<ChannelCreateDialog onCreated={onCreated} onClose={() => {}} />, { wrapper: makeWrapper() });

    await user.type(screen.getByLabelText("Name"), "Dupe");
    await user.type(screen.getByLabelText("Number"), "42");
    await user.click(screen.getByRole("button", { name: /create channel/i }));

    expect(await screen.findByText(/already in use/i)).toBeInTheDocument();
    expect(onCreated).not.toHaveBeenCalled();
  });
});
