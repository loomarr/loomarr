import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createMemoryHistory, createRouter, RouterProvider } from "@tanstack/react-router";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { routeTree } from "@/routeTree.gen";

// Router-level auth coverage (§11, §19): the _authed beforeLoad guard + the login flow,
// driven through the REAL generated route tree. `me` flips to 200 once a login POST
// lands, so the sign-in test exercises the guard re-running on navigation.
const ADMIN = { id: "u1", name: "Ada", role: "admin", autoApprove: true, disabled: false, quota: 0 };
const json = (body: unknown, status: number) =>
  new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });

const stubFetch = (startAuthed: boolean) => {
  let authed = startAuthed;
  const mock = vi.fn((url: string, _init?: RequestInit) => {
    const u = String(url);
    if (u.includes("/v1/auth/login")) {
      authed = true;
      return Promise.resolve(json(ADMIN, 200));
    }
    if (u.includes("/v1/auth/me")) {
      return Promise.resolve(authed ? json(ADMIN, 200) : json({ title: "Unauthorized" }, 401));
    }
    return Promise.resolve(json({}, 200));
  });
  vi.stubGlobal("fetch", mock);
  return mock;
};

const renderApp = (initialPath: string) => {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const router = createRouter({
    routeTree,
    context: { queryClient },
    history: createMemoryHistory({ initialEntries: [initialPath] }),
  });
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
};

afterEach(() => vi.restoreAllMocks());

describe("app router auth", () => {
  it("bounces a signed-out visitor from a protected route to the login form", async () => {
    stubFetch(false);
    renderApp("/channels");
    expect(await screen.findByLabelText("Username")).toBeInTheDocument();
  });

  it("renders the protected screen when already authenticated", async () => {
    stubFetch(true);
    renderApp("/channels");
    expect(await screen.findByRole("heading", { name: "Channels" })).toBeInTheDocument();
  });

  it("signs in and lands on the Channels home", async () => {
    const fetchMock = stubFetch(false);
    renderApp("/login");

    await userEvent.type(await screen.findByLabelText("Username"), "ada");
    await userEvent.type(screen.getByLabelText("Password"), "hunter2!");
    await userEvent.click(screen.getByRole("button", { name: /sign in/i }));

    expect(await screen.findByRole("heading", { name: "Channels" })).toBeInTheDocument();
    const posted = fetchMock.mock.calls.find(([u]) => String(u).includes("/v1/auth/login"));
    expect(posted?.[1]).toMatchObject({ method: "POST", credentials: "include" });
  });
});
