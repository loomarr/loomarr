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
    renderAt("/people");
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
    renderAt("/people");
    await screen.findByText("Grace");

    // Grace's row — the second Role select. Open it, then pick admin from its listbox
    // (only the opened Select mounts its options, so the query is unambiguous).
    const roles = screen.getAllByLabelText("Role");
    await userEvent.click(roles[1] as HTMLElement);
    await userEvent.click(await screen.findByRole("option", { name: "admin" }));

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
    renderAt("/people");
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
    renderAt("/people");
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
    renderAt("/people");
    await screen.findByText("Ada");
    expect(await screen.findByText(/connect emby or jellyfin/i)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /sync existing/i })).not.toBeInTheDocument();
  });

  // V7c: the admin half of local accounts. V7 shipped the endpoints; without these the
  // capability was reachable only by hand-crafting an API call — the same gap the
  // Account screen (V7b) closed on the self-service side.
  it("mounts the create-local-account panel and sends the typed role", async () => {
    const mock = stubFetch();
    renderAt("/people");
    await userEvent.click(await screen.findByRole("button", { name: /create local account/i }));
    await userEvent.type(screen.getByLabelText(/username/i), "newcomer");
    await userEvent.type(screen.getByLabelText(/^password$/i), "a-good-password");
    await userEvent.click(screen.getByRole("button", { name: /create account/i }));

    const call = mock.mock.calls.find(
      ([u, init]) => String(u).includes("/v1/users") && String((init as RequestInit)?.method) === "POST",
    );
    expect(call).toBeDefined();
    expect(JSON.parse(String((call?.[1] as RequestInit)?.body))).toEqual(
      expect.objectContaining({ username: "newcomer", password: "a-good-password", role: "member" }),
    );
  });

  it("offers Reset password on a local row and not on an imported one", async () => {
    // The row's local/media-server label has always existed to explain "whether a
    // password reset is even meaningful". This is that action — and the negative is the
    // point: Loomarr never held an imported user's credential, so offering to reset it
    // would imply it could change their media-server password.
    stubFetch();
    renderAt("/people");
    // Wait on a ROW affordance, not on "Ada" — that name also renders in the nav footer
    // as the signed-in user, so it resolves before the users query settles.
    await screen.findAllByRole("button", { name: /sessions/i });
    const resets = screen.getAllByRole("button", { name: /reset password/i });
    expect(resets).toHaveLength(1); // Ada is local; Grace is imported
  });

  it("sends the new password to the admin reset route", async () => {
    const mock = stubFetch();
    renderAt("/people");
    await screen.findAllByRole("button", { name: /sessions/i });
    await userEvent.click(screen.getByRole("button", { name: /reset password/i }));
    await userEvent.type(await screen.findByLabelText(/new password/i), "an-admin-set-pw");
    await userEvent.click(screen.getByRole("button", { name: /set new password/i }));

    const call = mock.mock.calls.find(([u]) => String(u).includes("/v1/users/u1/password"));
    expect(call).toBeDefined();
    expect(JSON.parse(String((call?.[1] as RequestInit)?.body))).toEqual({ next: "an-admin-set-pw" });
  });

  it("refuses the page to a member with an explanation, not failed requests", async () => {
    stubFetch({ me: { ...MEMBER } as typeof ADMIN });
    renderAt("/people");
    expect(await screen.findByText(/admins only/i)).toBeInTheDocument();
  });
});
