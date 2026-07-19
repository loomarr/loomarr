import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createMemoryHistory, createRouter, RouterProvider } from "@tanstack/react-router";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { routeTree } from "@/routeTree.gen";

const ADMIN = { id: "u1", name: "Ada", role: "admin", autoApprove: true, disabled: false, quota: 0 };
const MEMBER = { id: "u2", name: "Grace", role: "member", autoApprove: false, disabled: false, quota: 5 };

const USERS = [
  { ...ADMIN, local: true },
  { ...MEMBER, local: false },
];

const json = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });

type Opts = {
  me?: typeof ADMIN;
  userSync?: boolean;
  candidates?: Array<Record<string, unknown>>;
  sessions?: Array<Record<string, unknown>>;
};

const stubFetch = ({ me = ADMIN, userSync = true, candidates = [], sessions = [] }: Opts = {}) => {
  const mock = vi.fn((url: string, init?: RequestInit) => {
    const u = String(url);
    if (u.includes("/v1/auth/me")) return Promise.resolve(json(me));
    if (u.includes("/v1/users/candidates")) return Promise.resolve(json({ candidates }));
    if (u.includes("/sessions")) return Promise.resolve(json({ sessions }));
    if (u.includes("/v1/users/import")) return Promise.resolve(json({ imported: 1 }));
    if (u.includes("/v1/users/sync")) return Promise.resolve(json({ synced: 2 }));
    if (u.includes("/v1/users") && String(init?.method) === "PATCH") {
      return Promise.resolve(json(USERS[1]));
    }
    if (u.includes("/v1/users")) return Promise.resolve(json({ users: USERS }));
    if (u.includes("/v1/settings")) {
      return Promise.resolve(json({ features: { user_sync: userSync }, settings: [] }));
    }
    return Promise.resolve(json({}));
  });
  vi.stubGlobal("fetch", mock);
  vi.stubGlobal(
    "EventSource",
    class {
      addEventListener() {}
      close() {}
    },
  );
  return mock;
};

const renderAt = (path: string) => {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const router = createRouter({
    routeTree,
    context: { queryClient },
    history: createMemoryHistory({ initialEntries: [path] }),
  });
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
};

afterEach(() => vi.restoreAllMocks());

// These are PAGE-level assertions on purpose. Four times in this phase a component was
// built, unit-tested, and never actually mounted by any route — the component tests all
// passed while the feature was absent. Component tests pin behavior; these pin the wiring.
describe("Users page", () => {
  it("lists the allowlist with each row's credential path", async () => {
    stubFetch();
    renderAt("/users");
    // Await Grace, not Ada: the signed-in admin's name also renders in the app shell's
    // user menu, so awaiting "Ada" resolves before the table has loaded at all.
    expect(await screen.findByText("Grace")).toBeInTheDocument();
    expect(screen.getAllByText("Ada").length).toBeGreaterThan(0);
    // Exact strings, not regexes: a partial matcher also matches the ancestor <div>,
    // which contains the same text plus the user's name.
    expect(screen.getByText("Local account")).toBeInTheDocument();
    expect(screen.getByText("Media-server account")).toBeInTheDocument();
  });

  it("patches a single field without a save step", async () => {
    const fetchMock = stubFetch();
    renderAt("/users");
    await screen.findByText("Grace");

    // Grace's row — the second Role select.
    const roles = screen.getAllByLabelText("Role");
    await userEvent.selectOptions(roles[1] as HTMLElement, "admin");

    const patch = fetchMock.mock.calls.find(([, i]) => String(i?.method) === "PATCH");
    expect(patch, "changing a role should PATCH immediately").toBeDefined();
    expect(JSON.parse(String(patch?.[1]?.body))).toEqual({ role: "admin" });
  });

  it("opens a user's live sessions", async () => {
    stubFetch({
      sessions: [
        {
          id: "hash-a",
          userId: "u2",
          createdAt: Date.now() - 3_600_000,
          expiresAt: Date.now() + 3_600_000,
          current: false,
        },
      ],
    });
    renderAt("/users");
    await screen.findByText("Grace");

    const buttons = await screen.findAllByRole("button", { name: /sessions/i });
    await userEvent.click(buttons[1] as HTMLElement);
    expect(await screen.findByText(/1 active session for Grace/)).toBeInTheDocument();
  });

  // The import panel is the whole point of §11's "explicit import, never implicit". If it
  // is not on the page, an admin has no way to grant access at all.
  it("mounts the import panel with real candidate names", async () => {
    stubFetch({
      candidates: [
        { id: "e1", name: "Hopper", imported: false, disabled: false, isAdmin: true },
        { id: "e2", name: "Lovelace", imported: true, disabled: false, isAdmin: false },
      ],
    });
    renderAt("/users");
    expect(await screen.findByText("Hopper")).toBeInTheDocument();
    // Already-imported accounts stay visible, checked and locked — hiding them reads as
    // "missing", and re-offering them implies a no-op does something.
    expect(screen.getByText(/already imported/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/lovelace/i)).toBeDisabled();
  });

  // POST /v1/users/sync is registered only when a media server is configured, so the FE
  // must read the computed feature set first — otherwise it offers a call that 404s.
  it("explains rather than offering import when no media server is connected", async () => {
    stubFetch({ userSync: false });
    renderAt("/users");
    await screen.findByText("Ada");
    expect(await screen.findByText(/connect emby or jellyfin/i)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /sync existing/i })).not.toBeInTheDocument();
  });

  it("refuses the page to a member with an explanation, not failed requests", async () => {
    stubFetch({ me: { ...MEMBER } as typeof ADMIN });
    renderAt("/users");
    expect(await screen.findByText(/admins only/i)).toBeInTheDocument();
  });
});
